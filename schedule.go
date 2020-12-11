package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"
)

const (
	PredictTimeIndex    = 0
	PredictLatIndex     = 3
	PredictLonIndex     = 4
	PredictEclipseIndex = 5
	PredictSaaIndex     = 6
	PredictColumns      = 8
	PredictComma        = ','
	PredictComment      = '#'
)

const Leap = 18 * time.Second

const (
	DefaultDeltaTime = time.Second * 30
	Day              = time.Hour * 24
	Five             = time.Second * 5
)

type Entry struct {
	Label   string
	When    time.Time
	Warning bool
	Period
}

func (e Entry) IsZero() bool {
	return e.When.IsZero()
}

func SOY(t time.Time) int64 {
	year := t.AddDate(0, 0, -t.YearDay()+1).Truncate(Day)
	stamp := t.Add(Leap)
	return stamp.Unix() - year.Unix()
}

func (e Entry) SOY() int64 {
	return SOY(e.When)
}

type Schedule struct {
	Ignore   bool
	Eclipses []Period
	Saas     []Period
	Auroras  []Period
}

func Open(p string, d time.Duration, area Shape) (*Schedule, error) {
	r, err := os.Open(p)
	if err != nil {
		return nil, checkError(err, nil)
	}
	defer r.Close()
	return OpenReader(r, d, area)
}

func OpenReader(r io.Reader, d time.Duration, area Shape) (*Schedule, error) {
	var s Schedule
	return &s, s.listPeriods(r, d, area)
}

func (s *Schedule) Filter(t time.Time) *Schedule {
	if t.IsZero() {
		return s
	}
	var (
		es = make([]Period, 0, len(s.Eclipses))
		as = make([]Period, 0, len(s.Saas))
		xs = make([]Period, 0, len(s.Auroras))
	)
	for _, e := range s.Eclipses {
		if e.Starts.After(t) {
			es = append(es, e)
		}
	}
	for _, a := range s.Saas {
		if a.Starts.After(t) {
			as = append(as, a)
		}
	}
	for _, a := range s.Auroras {
		if a.Starts.After(t) {
			xs = append(xs, a)
		}
	}
	c := Schedule{
		Ignore:   s.Ignore,
		Eclipses: es,
		Saas:     as,
		Auroras:  xs,
	}
	return &c
}

func (s *Schedule) Periods() []Period {
	es := make([]Period, 0, len(s.Eclipses)+len(s.Saas)+len(s.Auroras))
	es = append(es, s.Eclipses...)
	es = append(es, s.Saas...)
	es = append(es, s.Auroras...)

	sort.Slice(es, func(i, j int) bool { return es[i].Starts.Before(es[j].Starts) })
	return es
}

func (s *Schedule) Schedule(roc RocOption, cer CerOption, aur AuroraOption) ([]Entry, error) {
	rs, err := s.ScheduleROC(roc)
	if err != nil {
		return nil, err
	}
	as, err := s.ScheduleCER(cer, roc, rs)
	if err != nil {
		return nil, err
	}
	cs, err := s.ScheduleACS(aur, roc, rs)
	if err != nil {
		return nil, err
	} else {
	}
	es := append([]Entry{}, rs...)
	es = append(es, as...)
	es = append(es, cs...)
	sort.Slice(es, func(i, j int) bool { return es[i].When.Before(es[j].When) })
	return es, nil
}

func (s *Schedule) ScheduleROC(roc RocOption) ([]Entry, error) {
	if roc.IsEmpty() {
		return nil, nil
	}
	return s.scheduleROC(roc)
}

func (s *Schedule) ScheduleCER(cer CerOption, roc RocOption, rs []Entry) ([]Entry, error) {
	if cer.IsEmpty() {
		return nil, nil
	}
	if cer.SwitchTime.IsZero() {
		if len(rs) == 0 {
			return nil, fmt.Errorf("CER: can not schedule without ROC")
		}
		return s.scheduleInsideCER(cer, roc, rs)
	}
	return s.scheduleOutsideCER(cer)
}

func (s *Schedule) ScheduleACS(aur AuroraOption, roc RocOption, rs []Entry) ([]Entry, error) {
	if aur.IsEmpty() {
		return nil, nil
	}
	var es []Entry
	if len(rs) == 0 {
		return nil, fmt.Errorf("ACS: can not schedule without ROC")
	}
	for _, p := range s.Auroras {
		if !aur.Accept(p) {
			continue
		}
		on := s.scheduleACSON(p, rs, aur, roc)
		if on.IsZero() {
			continue
		}
		// if between := aur.TimeBetween.Duration > 0 {
		// 	if diff := off.When.Sub(on.When.Add(aur.Time.Duration)); diff < between {
		// 		continue
		// 	}
		// }
		if off := s.scheduleACSOFF(p, aur, roc); !off.IsZero() && off.When.After(on.When.Add(aur.Time.Duration)) {
			es = append(es, on, off)
		}
	}
	return es, nil
}

func (s *Schedule) scheduleACSOFF(p Period, aur AuroraOption, roc RocOption) Entry {
	other := isCrossing(p, s.Eclipses, func(curr, other Period) bool {
		return !other.Ends.Before(curr.Ends.Add(-aur.Time.Duration))
	})
	e := Entry{
		Label: ACSOFF,
		Period: p,
	}
	if other.IsZero() {
		e.When = p.Ends.Add(-aur.Time.Duration)
		return e
	}
	if p.Ends.Add(aur.Time.Duration).Before(other.Ends.Add(-roc.TimeOff.Duration)) {
		e.When = p.Ends.Add(-aur.Time.Duration)
	}
	return e
}

func (s *Schedule) scheduleACSON(p Period, rs []Entry, aur AuroraOption, roc RocOption) Entry {
	var (
		min    = roc.TimeOn.Duration + roc.WaitBeforeOn.Duration
		starts = p.Starts.Add(-min)
		ends   = p.Starts.Add(min)
	)
	// schedule ACSON: try to find the nearset ROCON in its execution time
	// if no ROCON is found, ACSON can be scheduled at beginning of period
	// otherwise, ACSON should be scheduled at end of ROCON
	rocon := isNear(p, rs, func(e Entry) bool {
		if e.Label != ROCON {
			return false
		}
		return e.When.After(starts) && e.When.Before(ends)
	})
	e := Entry{
		Label: ACSON,
		Period: p,
	}
	if rocon.IsZero() {
		e.When = p.Starts
	} else {
		when := rocon.When.Add(roc.TimeOn.Duration)
		// when := rocon.When.Add(roc.TimeOn.Duration + roc.WaitBeforeOn.Duration)
		if when.After(p.Ends) {
			return e
		}
		e.When = when
	}
	return e
}

func (s *Schedule) scheduleInsideCER(cer CerOption, roc RocOption, rs []Entry) ([]Entry, error) {
	predicate := func(e, a Period) bool { return e.Overlaps(a) }

	var es []Entry
	for _, e := range s.Eclipses {
		as := isCrossingList(e, s.Saas, predicate)

		var p Period
		switch len(as) {
		case 0:
			continue
		case 1:
			p = as[0]
		default:
			f, t := as[0], as[len(as)-1]
			p = Period{
				Starts: f.Starts,
				Ends: t.Ends,
			}
		}
		if p.Duration() < cer.SaaCrossingTime.Duration || e.Intersect(p) < cer.SaaCrossingTime.Duration {
			continue
		}
		cn := Entry{
			Label: CERON,
			When:  p.Starts.Add(-cer.BeforeSaa.Duration),
			Period: p,
		}
		for i := len(rs) - 1; i >= 0; i-- {
			r := rs[i]
			var dr time.Duration
			switch r.Label {
			case ROCOFF:
				dr = roc.TimeOff.Duration
			case ROCON:
				dr = roc.TimeOn.Duration
			}
			if isBetween(r.When, r.When.Add(dr), cn.When) || isBetween(r.When, r.When.Add(dr), cn.When.Add(cer.TimeOn.Duration)) {
				cn.When = r.When.Add(-cer.BeforeRoc.Duration)
			}
		}
		cf := Entry{
			Label: CEROFF,
			When:  p.Ends.Add(cer.AfterSaa.Duration),
			Period: p,
		}
		for i := 0; i < len(rs); i++ {
			r := rs[i]

			var dr time.Duration
			switch r.Label {
			case ROCOFF:
				dr = roc.TimeOff.Duration
			case ROCON:
				dr = roc.TimeOn.Duration
			}
			if isBetween(r.When, r.When.Add(dr), cf.When) || isBetween(r.When, r.When.Add(dr), cf.When.Add(cer.TimeOff.Duration)) {
				cf.When = r.When.Add(dr + cer.AfterRoc.Duration)
			}
		}
		es = append(es, cn, cf)
	}
	return es, nil
}

func (s *Schedule) scheduleOutsideCER(cer CerOption) ([]Entry, error) {
	eclipses := make([]Period, len(s.Eclipses))
	copy(eclipses, s.Eclipses)

	var (
		crossing bool
		es       []Entry
	)
	predicate := func(e, a Period) bool {
		return cer.SaaCrossingTime.IsZero() || e.Intersect(a) > cer.SaaCrossingTime.Duration
	}
	for len(eclipses) > 0 {
		e := eclipses[0]
		if a := isCrossing(e, s.Saas, predicate); !a.IsZero() {
			crossing = true
			es = append(es, Entry{
				Label: CERON,
				When:  e.Starts.Add(-cer.TimeOn.Duration),
			})
		} else {
			crossing = false
			es = append(es, Entry{
				Label: CEROFF,
				When:  e.Starts.Add(-cer.TimeOff.Duration),
				Period: e,
			})
		}
		eclipses = skipEclipses(eclipses[1:], s.Saas, crossing, cer.SaaCrossingTime.Duration)
	}
	return es, nil
}

func (s *Schedule) scheduleROC(roc RocOption) ([]Entry, error) {
	var (
		es        []Entry
		predicate = func(e, a Period) bool { return e.Overlaps(a) }
	)

	for _, e := range s.Eclipses {
		as := isCrossingList(e, s.Saas, predicate)
		var s1, s2 Period
		switch z := len(as); {
		case z == 0:
		case z == 1:
			s1, s2 = as[0], as[0]
		default:
			s1, s2 = as[0], as[z-1]
		}
		var (
			rocon  = scheduleROCON(e, s1, roc)
			rocoff = scheduleROCOFF(e, s2, roc)
		)

		if !roc.TimeBetween.IsZero() && rocoff.When.Sub(rocon.When.Add(roc.TimeOn.Duration)) <= roc.TimeBetween.Duration {
			if !s.Ignore {
				continue
			}
			rocon.Warning, rocoff.Warning = true, true
		}
		if rocoff.When.Before(rocon.When) || rocoff.When.Sub(rocon.When) <= roc.TimeOn.Duration {
			if !s.Ignore {
				continue
			}
			rocon.Warning, rocoff.Warning = true, true
		}
		es = append(es, rocon, rocoff)
	}
	return es, nil
}

func scheduleROCON(e, s Period, roc RocOption) Entry {
	y := Entry{
		Label: ROCON,
		When:  e.Starts.Add(roc.WaitBeforeOn.Duration),
		Period: e,
	}
	if s.IsZero() {
		return y
	}
	if !roc.TimeSAA.IsZero() && s.Duration() <= roc.TimeSAA.Duration {
		enter, exit := s.Starts, s.Starts.Add(2*roc.TimeAZM.Duration)
		if isBetween(enter, exit, y.When) || isBetween(enter, exit, y.When.Add(roc.TimeOn.Duration)) {
			y.When = exit
		}
		return y
	}
	// check that ROCON does not completly overlap AZM of SAA enter
	// then check that ROCON does not start within the AZM of the SAA enter
	if y.When.Before(s.Starts) && y.When.Add(roc.TimeOn.Duration).After(s.Starts.Add(roc.TimeAZM.Duration)) {
		y.When = s.Starts.Add(roc.TimeAZM.Duration)
	}
	if isBetween(s.Starts, s.Starts.Add(roc.TimeAZM.Duration), y.When) || isBetween(s.Starts, s.Starts.Add(roc.TimeAZM.Duration), y.When.Add(roc.TimeOn.Duration)) {
		y.When = s.Starts.Add(roc.TimeAZM.Duration)
	}
	// check that ROCON does not completly overlap AZM of SAA exit
	// then check that ROCON does not start within the AZM of the SAA exit
	if y.When.Before(s.Ends) && y.When.Add(roc.TimeOn.Duration).After(s.Ends.Add(roc.TimeAZM.Duration)) {
		y.When = s.Ends.Add(roc.TimeAZM.Duration)
	}
	if isBetween(s.Ends, s.Ends.Add(roc.TimeAZM.Duration), y.When) || isBetween(s.Ends, s.Ends.Add(roc.TimeAZM.Duration), y.When.Add(roc.TimeOn.Duration-time.Second)) {
		y.When = s.Ends.Add(roc.TimeAZM.Duration)
	}
	return y
}

func scheduleROCOFF(e, s Period, roc RocOption) Entry {
	y := Entry{
		Label: ROCOFF,
		When:  e.Ends.Add(-roc.TimeOff.Duration),
		Period: e,
	}
	if s.IsZero() {
		return y
	}
	if roc.TimeSAA.Duration > 0 && s.Duration() <= roc.TimeSAA.Duration {
		enter, exit := s.Starts, s.Starts.Add(2*roc.TimeAZM.Duration)
		if isBetween(enter, exit, y.When) || isBetween(enter, exit, y.When.Add(roc.TimeOff.Duration)) {
			y.When = enter.Add(-roc.TimeOff.Duration)
		}
		return y
	}
	// check that ROCOFF does not completly overlap AZM of SAA exit
	// then check that ROCOFF does not start within the AZM of the SAA exit
	if y.When.Before(s.Ends) && y.When.Add(roc.TimeOff.Duration).After(s.Ends.Add(roc.TimeAZM.Duration)) {
		y.When = s.Ends.Add(roc.TimeAZM.Duration)
	}
	if isBetween(s.Ends, s.Ends.Add(roc.TimeAZM.Duration), y.When) || isBetween(s.Ends, s.Ends.Add(roc.TimeAZM.Duration), y.When.Add(roc.TimeOff.Duration)) {
		y.When = s.Ends.Add(-roc.TimeOff.Duration)
	}
	// check that ROCON does not completly overlap AZM of SAA enter
	// then check that ROCON does not start within the AZM of the SAA enter
	if y.When.Before(s.Starts) && y.When.Add(roc.TimeOff.Duration).After(s.Starts.Add(roc.TimeAZM.Duration)) {
		y.When = s.Starts.Add(-roc.TimeOff.Duration)
	}
	if isBetween(s.Starts, s.Starts.Add(roc.TimeAZM.Duration-time.Second), y.When) || isBetween(s.Starts, s.Starts.Add(roc.TimeAZM.Duration), y.When.Add(roc.TimeOff.Duration)) {
		y.When = s.Starts.Add(-roc.TimeOff.Duration)
	}
	return y
}

func isBetween(f, t, d time.Time) bool {
	return f.Before(t) && (f.Equal(d) || t.Equal(d) || f.Before(d) && t.After(d))
}

func (s *Schedule) listPeriods(r io.Reader, resolution time.Duration, area Shape) error {
	rs := csv.NewReader(r)
	rs.Comment = PredictComment
	rs.Comma = PredictComma
	rs.FieldsPerRecord = PredictColumns

	if r, err := rs.Read(); r == nil && err != nil {
		return err
	}

	var e, a, x, z Period
	for i := 0; ; i++ {
		r, err := rs.Read()
		if r == nil && err == io.EOF {
			break
		}
		if err != nil {
			return badUsage(err.Error())
		}
		lat, lng, err := parseLatLng(r, i)
		if err != nil {
			return err
		}
		if area.Contains(lat, lng) && isEnterPeriod(r[PredictEclipseIndex]) && x.IsZero() {
			if x.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if (!area.Contains(lat, lng) || isLeavePeriod(r[PredictEclipseIndex])) && !x.IsZero() {
			if x.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
			s.Auroras = append(s.Auroras, Period{
				Label:  "aurora",
				Starts: x.Starts.UTC(),
				Ends:   x.Ends.Add(-resolution).UTC(),
			})
			x = z
		}
		if isEnterPeriod(r[PredictEclipseIndex]) && e.IsZero() {
			if e.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictEclipseIndex]) && !e.IsZero() {
			if e.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
			s.Eclipses = append(s.Eclipses, Period{
				Label:  "eclipse",
				Starts: e.Starts.UTC(),
				Ends:   e.Ends.Add(-resolution).UTC(),
			})
			e = z
		}
		if isEnterPeriod(r[PredictSaaIndex]) && a.IsZero() {
			if a.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictSaaIndex]) && !a.IsZero() {
			if a.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return timeBadSyntax(i, r[PredictTimeIndex])
			}
			s.Saas = append(s.Saas, Period{
				Label:  "saa",
				Starts: a.Starts.UTC(),
				Ends:   a.Ends.Add(-resolution).UTC(),
			})
			a = z
		}
	}
	if len(s.Eclipses) == 0 && len(s.Saas) == 0 {
		return fmt.Errorf("no eclipses/saas found")
	}
	sort.Slice(s.Eclipses, func(i, j int) bool { return s.Eclipses[i].Starts.Before(s.Eclipses[j].Starts) })
	sort.Slice(s.Saas, func(i, j int) bool { return s.Saas[i].Starts.Before(s.Saas[j].Starts) })
	sort.Slice(s.Auroras, func(i, j int) bool { return s.Auroras[i].Starts.Before(s.Auroras[j].Starts) })
	return nil
}

func parseLatLng(r []string, i int) (float64, float64, error) {
	lat, err := strconv.ParseFloat(r[PredictLatIndex], 64)
	if err != nil {
		return 0, 0, floatBadSyntax(i, r[PredictLatIndex])
	}
	lng, err := strconv.ParseFloat(r[PredictLonIndex], 64)
	if err != nil {
		return 0, 0, floatBadSyntax(i, r[PredictLonIndex])
	}
	return lat, lng, err
}

func isEnterPeriod(r string) bool {
	return r == "1" || r == "true" || r == "on"
}

func isLeavePeriod(r string) bool {
	return r == "0" || r == "false" || r == "off"
}

func skipEclipses(es, as []Period, cross bool, d time.Duration) []Period {
	predicate := func(e, a Period) bool {
		return d == 0 || e.Intersect(a) > d
	}
	for i, e := range es {
		switch a := isCrossing(e, as, predicate); {
		case cross && !a.IsZero():
		case !cross && a.IsZero():
		default:
			return es[i:]
		}
	}
	return nil
}

func isNear(a Period, es []Entry, predicate func(Entry) bool) Entry {
	var y Entry
	for _, e := range es {
		if predicate(e) {
			y = e
			break
		}
		if e.When.After(a.Ends) {
			break
		}
	}
	return y
}

type PeriodFunc func(Period, Period) bool

func isCrossingList(e Period, as []Period, predicate PeriodFunc) []Period {
	var es []Period
	for _, a := range as {
		if predicate(e, a) {
			es = append(es, a)
		}
		if a.Starts.After(e.Ends) {
			break
		}
	}
	return es
}

func isCrossing(e Period, as []Period, predicate PeriodFunc) Period {
	var p Period
	if len(as) == 0 {
		return p
	}
	for _, a := range as {
		if predicate(e, a) {
			p = a
			break
		}
		if a.Starts.After(e.Ends) {
			break
		}
	}
	return p
}

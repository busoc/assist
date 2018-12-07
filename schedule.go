package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

const (
	PredictTimeIndex    = 0
	PredictEclipseIndex = 5
	PredictSaaIndex     = 6
	PredictColumns      = 8
	PredictComma        = ','
	PredictComment      = '#'
)

const Leap = 18 * time.Second

const (
	DefaultDeltaTime     = time.Second * 30
	DefaultIntersectTime = time.Second * 120
	Day                  = time.Hour * 24
	Five                 = time.Second * 5
)

type Entry struct {
	Label   string
	When    time.Time
	Warning bool
}

func SOY(t time.Time) int64 {
	year := t.AddDate(0, 0, -t.YearDay()+1).Truncate(Day).Add(Leap)
	stamp := t.Add(Leap)
	return stamp.Unix() - year.Unix()
}

func (e Entry) SOY() int64 {
	return SOY(e.When)
}

type Schedule struct {
	Ignore   bool
	Eclipses []*Period
	Saas     []*Period
}

func Open(p string, d time.Duration) (*Schedule, error) {
	r, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return OpenReader(r, d)
}

func OpenReader(r io.Reader, d time.Duration) (*Schedule, error) {
	var (
		s   Schedule
		err error
	)
	s.Eclipses, s.Saas, err = listPeriods(r, d)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Schedule) Filter(t time.Time) *Schedule {
	if t.IsZero() {
		return s
	}
	es, as := make([]*Period, 0, len(s.Eclipses)), make([]*Period, 0, len(s.Saas))
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
	return &Schedule{Ignore: s.Ignore, Eclipses: es, Saas: as}
}

func (s *Schedule) Periods() []*Period {
	es := make([]*Period, len(s.Eclipses)+len(s.Saas))
	for i := 0; i < len(s.Eclipses); i++ {
		es[i] = s.Eclipses[i]
	}
	for i := len(s.Eclipses); i < len(es); i++ {
		es[i] = s.Saas[i-len(s.Eclipses)]
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Starts.Before(es[j].Starts) })
	return es
}

func (s *Schedule) Schedule(d delta, roc, cer bool) ([]*Entry, error) {
	if !roc && !cer {
		return nil, nil
	}
	var es []*Entry
	if roc {
		if vs, err := s.scheduleMXGS(d.Rocon.Duration, d.Rocoff.Duration, d.Wait.Duration, d.AZM.Duration, d.Margin.Duration, d.Saa.Duration); err != nil {
			return nil, err
		} else {
			es = append(es, vs...)
		}
	}
	if cer {
		if vs, err := s.scheduleMMIA(d.Cer.Duration, d.Intersect.Duration); err != nil {
			return nil, err
		} else {
			es = append(es, vs...)
		}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].When.Before(es[j].When) })
	return es, nil
}

func (s *Schedule) scheduleMXGS(on, off, wait, azm, margin, saa time.Duration) ([]*Entry, error) {
	predicate := func(e, a *Period) bool { return e.Overlaps(a) }
	var es []*Entry

	for _, e := range s.Eclipses {
		as := isCrossingList(e, s.Saas, predicate)
		// if len(as) > 0 && e.Duration() <= on+off+margin+wait {
		// 	log.Println("skip due to eclipse duration", e.Starts, e.Ends, e.Duration(), on+off+margin+wait)
		// 	continue
		// }
		var s1, s2 *Period
		switch z := len(as); {
		case z == 0:
		case z == 1:
			s1, s2 = as[0], as[0]
		default:
			s1, s2 = as[0], as[z-1]
		}
		rocon := scheduleROCON(e, s1, on, wait, azm, saa)
		rocoff := scheduleROCOFF(e, s2, off, azm, saa)

		if margin > 0 && rocoff.When.Sub(rocon.When.Add(on)) <= margin {
			if !s.Ignore {
				continue
			}
			rocon.Warning, rocoff.Warning = true, true
		}
		if rocoff.When.Before(rocon.When) || rocoff.When.Sub(rocon.When) <= on {
			if !s.Ignore {
				continue
			}
			rocon.Warning, rocoff.Warning = true, true
		}
		es = append(es, rocon, rocoff)
	}
	return es, nil
}

func (s *Schedule) scheduleMMIA(delta, intersect time.Duration) ([]*Entry, error) {
	eclipses := make([]*Period, len(s.Eclipses))
	copy(eclipses, s.Eclipses)

	var (
		crossing bool
		es       []*Entry
	)
	predicate := func(e, a *Period) bool {
		return intersect == 0 || e.Intersect(a) > intersect
	}
	for len(eclipses) > 0 {
		e := eclipses[0]
		if a := isCrossing(e, s.Saas, predicate); a != nil {
			crossing = true
			es = append(es, &Entry{Label: CERON, When: e.Starts.Add(-delta)})
		} else {
			crossing = false
			es = append(es, &Entry{Label: CEROFF, When: e.Starts.Add(-delta)})
		}
		eclipses = skipEclipses(eclipses[1:], s.Saas, crossing, intersect)
	}
	return es, nil
}

type Period struct {
	Label        string
	Starts, Ends time.Time
}

func (p Period) Duration() time.Duration {
	return p.Ends.Sub(p.Starts)
}

func (p Period) IsZero() bool {
	return p.Starts.IsZero() && p.Ends.IsZero()
}

func (p Period) Contains(o *Period) bool {
	if o.Starts.Before(p.Starts) {
		return false
	}
	return o.Starts.Add(o.Duration()).Before(p.Ends)
}

func (p Period) Overlaps(o *Period) bool {
	return !(o.Starts.After(p.Ends) || o.Ends.Before(p.Starts))
}

func (p Period) Intersect(o *Period) time.Duration {
	if !p.Overlaps(o) {
		return 0
	}
	if p.Contains(o) {
		return o.Duration()
	}
	var delta time.Duration
	if p.Starts.After(o.Starts) {
		delta = o.Ends.Sub(p.Starts)
	} else {
		delta = p.Ends.Sub(o.Starts)
	}
	return delta
}

func scheduleROCON(e, s *Period, on, wait, azm, saa time.Duration) *Entry {
	y := Entry{Label: ROCON, When: e.Starts.Add(wait)}
	if s == nil {
		return &y
	}
	if saa > 0 && s.Duration() <= saa {
		enter, exit := s.Starts, s.Starts.Add(2*azm)
		if isBetween(enter, exit, y.When) || isBetween(enter, exit, y.When.Add(on)) {
			y.When = exit
		}
		return &y
	}
	if isBetween(s.Starts, s.Starts.Add(azm), y.When) || isBetween(s.Starts, s.Starts.Add(azm), y.When.Add(on)) {
		y.When = s.Starts.Add(azm)
	}
	if isBetween(s.Ends, s.Ends.Add(azm), y.When) || isBetween(s.Ends, s.Ends.Add(azm), y.When.Add(on-time.Second)) {
		y.When = s.Ends.Add(azm)
	}
	return &y
}

func scheduleROCOFF(e, s *Period, off, azm, saa time.Duration) *Entry {
	y := Entry{Label: ROCOFF, When: e.Ends.Add(-off)}
	if s == nil {
		return &y
	}
	if saa > 0 && s.Duration() <= saa {
		enter, exit := s.Starts, s.Starts.Add(2*azm)
		if isBetween(enter, exit, y.When) || isBetween(enter, exit, y.When.Add(off)) {
			y.When = enter.Add(-off)
		}
		return &y
	}
	if isBetween(s.Ends, s.Ends.Add(azm), y.When) || isBetween(s.Ends, s.Ends.Add(azm), y.When.Add(off)) {
		y.When = s.Ends.Add(-off)
	}
	if isBetween(s.Starts, s.Starts.Add(azm-time.Second), y.When) || isBetween(s.Starts, s.Starts.Add(azm), y.When.Add(off)) {
		y.When = s.Starts.Add(-off)
	}
	return &y
}

func isBetween(f, t, d time.Time) bool {
	return f.Before(t) && (f.Equal(d) || t.Equal(d) || f.Before(d) && t.After(d))
}

func listPeriods(r io.Reader, resolution time.Duration) ([]*Period, []*Period, error) {
	rs := csv.NewReader(r)
	rs.Comment = PredictComment
	rs.Comma = PredictComma
	rs.FieldsPerRecord = PredictColumns

	if r, err := rs.Read(); r == nil && err != nil {
		return nil, nil, err
	}

	var (
		e, a, z Period
		es, as  []*Period
	)
	for i := 0; ; i++ {
		r, err := rs.Read()
		if r == nil && err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if isEnterPeriod(r[PredictEclipseIndex]) && e.IsZero() {
			if e.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictEclipseIndex]) && !e.IsZero() {
			if e.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
			es = append(es, &Period{
				Label:  "eclipse",
				Starts: e.Starts.UTC(),
				Ends:   e.Ends.Add(-resolution).UTC(),
			})
			e = z
		}
		if isEnterPeriod(r[PredictSaaIndex]) && a.IsZero() {
			if a.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictSaaIndex]) && !a.IsZero() {
			if a.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
			as = append(as, &Period{
				Label:  "saa",
				Starts: a.Starts.UTC(),
				Ends:   a.Ends.Add(-resolution).UTC(),
			})
			a = z
		}
	}
	if len(es) == 0 && len(as) == 0 {
		return nil, nil, fmt.Errorf("no eclipses/saas found")
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Starts.Before(es[j].Starts) })
	sort.Slice(as, func(i, j int) bool { return as[i].Starts.Before(as[j].Starts) })
	return es, as, nil
}

func timeBadSyntax(i int, v string) error {
	return fmt.Errorf("time badly formatted at row %d (%s)", i+1, v)
}

func isEnterPeriod(r string) bool {
	return r == "1" || r == "true" || r == "on"
}

func isLeavePeriod(r string) bool {
	return r == "0" || r == "false" || r == "off"
}

func skipEclipses(es, as []*Period, cross bool, d time.Duration) []*Period {
	predicate := func(e, a *Period) bool {
		return d == 0 || e.Intersect(a) > d
	}
	for i, e := range es {
		switch a := isCrossing(e, as, predicate); {
		case cross && a != nil:
		case !cross && a == nil:
		default:
			return es[i:]
		}
	}
	return nil
}

func isCrossingList(e *Period, as []*Period, predicate func(*Period, *Period) bool) []*Period {
	if len(as) == 0 {
		return nil
	}
	var es []*Period
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

func isCrossing(e *Period, as []*Period, predicate func(*Period, *Period) bool) *Period {
	if len(as) == 0 {
		return nil
	}
	for _, a := range as {
		if predicate(e, a) {
			return a
		}
		if a.Starts.After(e.Ends) {
			break
		}
	}
	return nil
}

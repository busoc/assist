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

type Entry struct {
	When  time.Time
	Label string
}

type Schedule struct {
  Eclipses []*Period
  Saas     []*Period
}

func Open(p string, r time.Duration) (*Schedule, error) {
  var (
    s Schedule
    err error
  )
  s.Eclipses, s.Saas, err = listPeriods(p, r)
  if err != nil {
    return nil, err
  }
  return &s, nil
}

func (s *Schedule) Filter(t time.Time) *Schedule {
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
	return &Schedule{es, as}
}

func (s *Schedule) Schedule(d delta) ([]*Entry, error) {
  var es []*Entry
	if vs, err := s.scheduleMXGS(d.Rocon, d.Rocoff, d.Wait, d.AZM); err != nil {
		return nil, err
	} else {
		es = append(es, vs...)
	}
	if vs, err := s.scheduleMMIA(d.Cer, d.Intersect); err != nil {
		return nil, err
	} else {
		es = append(es, vs...)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].When.Before(es[j].When) })
  return es, nil
}

func (s *Schedule) scheduleMXGS(on, off, wait, azm time.Duration) ([]*Entry, error) {
	predicate := func(e, a *Period) bool { return e.Overlaps(a) }
	var es []*Entry
	for _, e := range s.Eclipses {
		if e.Duration() <= on+off+wait {
			continue
		}
		as := isCrossingList(e, s.Saas, predicate)
		var s1, s2 *Period
		switch z := len(as); {
		case z == 0:
		case z == 1:
			s1, s2 = as[0], as[0]
		default:
			s1, s2 = as[0], as[z-1]
		}
		rocon := scheduleROCON(e, s1, on, wait, azm)
		rocoff := scheduleROCOFF(e, s2, off, azm)

		if rocoff.When.Before(rocon.When) || rocoff.When.Sub(rocon.When) <= on {
			continue
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

func scheduleROCON(e, s *Period, on, wait, azm time.Duration) *Entry {
	start := e.Starts.Add(wait)
	end := start.Add(on)

	y := &Entry{Label: ROCON, When: start}
	// no SAA crossing
	if s == nil {
		return y
	}
	if z := s.Starts.Add(azm); z.After(start) && s.Starts.Before(end) {
		y.When = z
		if s.Ends.Sub(y.When) < on {
			y.When = s.Ends.Add(azm)
		}
		return y
	}
	// if z := s.Starts.Add(azm); isBetween(start, end, s.Starts) || isBetween(start, end, z) {
	// 	y.When = z
	// 	if s.Ends.Sub(y.When) < on {
	// 		y.When = s.Ends.Add(azm)
	// 	}
	// 	return y
	// }
	if z := s.Ends.Add(azm); z.After(start) && s.Ends.Before(end) {
		y.When = z
	}
	// if z := s.Ends.Add(azm); isBetween(start, end, s.Ends) || isBetween(start, end, z) {
	// 	y.When = z
	// }
	return y
}

func scheduleROCOFF(e, s *Period, off, azm time.Duration) *Entry {
	start := e.Ends.Add(-off)
	end := e.Ends

	y := &Entry{Label: ROCOFF, When: start}
	if s == nil {
		return y
	}
	if z := s.Starts.Add(azm); z.After(start) && s.Starts.Before(end) {
		y.When = s.Starts.Add(-off)
		return y
	}
	// if z := s.Starts.Add(azm); isBetween(start, end, s.Starts) || isBetween(start, end, z) {
	// 	y.When = s.Starts.Add(-off)
	// 	return y
	// }
	if z := s.Ends.Add(azm); z.After(start) && s.Ends.Before(end) {
		y.When = s.Starts.Add(-off)
		return y
	}
	// if z := s.Ends.Add(azm); isBetween(start, end, s.Ends) || isBetween(start, end, z) {
	// 	y.When = s.Ends.Add(-off)
	// 	return y
	// }
	return y
}

func isBetween(f, t, d time.Time) bool {
	return f.Before(d) && t.After(d)
}

func listPeriods(file string, resolution time.Duration) ([]*Period, []*Period, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	rs := csv.NewReader(r)
	rs.Comment = PredictComment
	rs.Comma = PredictComma
	rs.FieldsPerRecord = PredictColumns

	if r, err := rs.Read(); r == nil && err != nil {
		return nil, nil, err
	}

	var (
		e, a, z Period
		es, as []*Period
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
			es = append(es, &Period{e.Starts.UTC(), e.Ends.Add(-resolution).UTC()})
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
			as = append(as, &Period{a.Starts.UTC(), a.Ends.Add(-resolution).UTC()})
			a = z
		}
	}
	if len(es) == 0 && len(as) == 0 {
		return nil, nil, fmt.Errorf("no eclipses/saas found")
	}
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

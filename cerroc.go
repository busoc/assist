package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const Leap = 18 * time.Second

const (
	Version   = "0.1.0-beta"
	BuildTime = "2018-09-26 17:10:00"
	Program   = "assist"
)

const (
	DefaultDeltaTime     = time.Second * 30
	DefaultIntersectTime = time.Second * 120
	Ninety               = time.Second * 90
	Day                  = time.Hour * 24
	Five                 = time.Second * 5
)

const (
	ROCON  = "ROCON"
	ROCOFF = "ROCOFF"
	CERON  = "CERON"
	CEROFF = "CEROFF"
)

type Entry struct {
	When  time.Time
	Label string
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

type Timeline struct {
	Eclipses []*Period
	Saas     []*Period
}

func (t *Timeline) Schedule(d delta) (*Schedule, error) {
	var es []*Entry
	if vs, err := t.scheduleMXGS(d.Rocon, d.Rocoff, time.Minute*5, d.AZM); err != nil {
		return nil, err
	} else {
		es = append(es, vs...)
	}
	// if vs, err := t.scheduleMMIA(d.Cer, d.Intersect); err != nil {
	// 	return nil, err
	// } else {
	// 	es = append(es, vs...)
	// }
	sort.Slice(es, func(i, j int) bool { return es[i].When.Before(es[j].When) })

	return &Schedule{When: es[0].When.Add(-time.Second * 5).Truncate(time.Second), Entries: es}, nil
}

func (t *Timeline) scheduleMXGS(on, off, min, azm time.Duration) ([]*Entry, error) {
	predicate := func(e, a *Period) bool { return e.Overlaps(a) }
	var es []*Entry
	for _, e := range t.Eclipses {
		if e.Duration() < min {
			continue
		}
		s := isCrossing(e, t.Saas, predicate)
		//ROC schedule entry
		rocon, rocoff := &Entry{Label: ROCON}, &Entry{Label: ROCOFF}
		if s != nil {
			// ROCON
			switch when := e.Starts.Add(Ninety); {
			case s.Ends.Add(azm).Before(when) || when.Add(on).Before(s.Starts):
				// exit SAA before ROCON starts or enter SAA after ROCON fully executed
				rocon.When = when // nominal
			case 
			default:
				rocon.When = when // nominal
			}
			// ROCOFF
			switch when := e.Ends.Add(-off); {
			default:
				rocoff.When = when // nominal
			}
		} else {
			rocon.When = e.Starts.Add(Ninety)
			rocoff.When = e.Ends.Add(-off)
		}
		if rocoff.When.Sub(rocon.When) < time.Minute {
			continue
		}
		es = append(es, rocon, rocoff)
	}
	return es, nil
}

func (t *Timeline) scheduleMMIA(delta, intersect time.Duration) ([]*Entry, error) {
	eclipses := make([]*Period, len(t.Eclipses))
	copy(eclipses, t.Eclipses)

	var (
		crossing bool
		es       []*Entry
	)
	predicate := func(e, a *Period) bool {
		return intersect == 0 || e.Intersect(a) > intersect
	}
	for len(eclipses) > 0 {
		e := eclipses[0]
		if a := isCrossing(e, t.Saas, predicate); a != nil {
			crossing = true
			es = append(es, &Entry{Label: CERON, When: e.Starts.Add(-delta)})
		} else {
			crossing = false
			es = append(es, &Entry{Label: CEROFF, When: e.Starts.Add(-delta)})
		}
		eclipses = skipEclipses(eclipses[1:], t.Saas, crossing, intersect)
	}
	return es, nil
}

type delta struct {
	Rocon     time.Duration
	Rocoff    time.Duration
	Cer       time.Duration
	Intersect time.Duration
	AZM       time.Duration
}

type Schedule struct {
	When    time.Time
	Entries []*Entry
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}

const (
	PredictTimeIndex    = 0
	PredictEclipseIndex = 5
	PredictSaaIndex     = 6
	PredictColumns      = 8
	PredictComma        = ','
	PredictComment      = '#'
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ASIM semi automatic schedule generator tool\n")
		fmt.Fprintf(os.Stderr, "assist [-r] [-z] [-delta-rocon] [-delta-rocoff] [-delta-cer] [-i] <predict>\n")
		os.Exit(2)
	}
	var d delta
	flag.DurationVar(&d.Rocon, "delta-rocon", 50*time.Second, "delta ROC margin time (10s)")
	flag.DurationVar(&d.Rocoff, "delta-rocoff", 80*time.Second, "delta ROC margin time (80s)")
	flag.DurationVar(&d.Cer, "delta-cer", DefaultDeltaTime, "delta CER margin time (30s)")
	flag.DurationVar(&d.Intersect, "i", DefaultIntersectTime, "intersection time (2m)")
	flag.DurationVar(&d.AZM, "z", DefaultDeltaTime, "default AZM duration (30s)")
	resolution := flag.Duration("r", time.Second*10, "prediction accuracy (10s)")
	flag.Parse()

	ts, err := listPeriods(flag.Arg(0), *resolution)
	if err != nil {
		log.Fatalln(err)
	}
	if ts == nil {
		log.Println("no records found in %s", flag.Arg(0))
		return
	}
	s, err := ts.Schedule(d)
	if err != nil {
		log.Fatalln(err)
	}
	for i, e := range s.Entries {
		log.Printf("%3d | %7s | %s", i+1, e.Label, e.When.Truncate(time.Second).Format(timeFormat))
	}
}

func listPeriods(file string, resolution time.Duration) (*Timeline, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	rs := csv.NewReader(r)
	rs.Comment = PredictComment
	rs.Comma = PredictComma
	rs.FieldsPerRecord = PredictColumns

	if r, err := rs.Read(); r == nil && err != nil {
		return nil, err
	}

	var (
		e, a, z Period
		ts      Timeline
	)
	for i := 0; ; i++ {
		r, err := rs.Read()
		if r == nil && err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if isEnterPeriod(r[PredictEclipseIndex]) && e.IsZero() {
			if e.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictEclipseIndex]) && !e.IsZero() {
			if e.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
			ts.Eclipses = append(ts.Eclipses, &Period{e.Starts.UTC(), e.Ends.Add(-resolution).UTC()})
			e = z
		}
		if isEnterPeriod(r[PredictSaaIndex]) && a.IsZero() {
			if a.Starts, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
		}
		if isLeavePeriod(r[PredictSaaIndex]) && !a.IsZero() {
			if a.Ends, err = time.Parse(timeFormat, r[PredictTimeIndex]); err != nil {
				return nil, timeBadSyntax(i, r[PredictTimeIndex])
			}
			ts.Saas = append(ts.Saas, &Period{a.Starts.UTC(), a.Ends.Add(-resolution).UTC()})
			a = z
		}
	}
	return &ts, nil
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

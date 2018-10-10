package main

import (
	"bufio"
	"crypto/md5"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
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

func (t *Timeline) Schedule(d delta, base time.Time) (*Schedule, error) {
	var es []*Entry
	if vs, err := t.scheduleMXGS(d.Rocon, d.Rocoff, 0, d.AZM); err != nil {
		return nil, err
	} else {
		es = append(es, vs...)
	}
	if vs, err := t.scheduleMMIA(d.Cer, d.Intersect); err != nil {
		return nil, err
	} else {
		es = append(es, vs...)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].When.Before(es[j].When) })

	return &Schedule{When: base.Add(-time.Second * 5).Truncate(time.Second), Entries: es}, nil
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
			//ROCOFF
			switch {
			case e.Ends.After(s.Ends.Add(azm)) && e.Ends.Add(azm).Sub(s.Ends) < off:
				rocoff.When = s.Ends.Add(-off)
			case e.Ends.After(s.Starts.Add(azm)) && e.Ends.Sub(s.Starts.Add(azm)) < off:
				rocoff.When = s.Starts.Add(azm).Add(-off)
			default:
				rocoff.When = e.Ends.Add(-off)
			}
			//ROCON
			switch n := e.Starts.Add(Ninety); {
			case s.Starts.After(n) && s.Starts.Sub(n) < on:
				rocon.When = s.Starts.Add(Ninety + on)
			case e.Ends.After(n) && s.Ends.Sub(n) < on:
				rocon.When = s.Ends.Add(Ninety + on)
			default:
				rocon.When = n.Add(on)
			}
		} else {
			rocon.When = e.Starts.Add(Ninety + on)
			rocoff.When = e.Ends.Add(-off)
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

type fileset struct {
	Rocon  string
	Rocoff string
	Ceron  string
	Ceroff string
	Keep   bool
}

func (f fileset) CanROC() bool {
	return f.Rocon != "" && f.Rocoff != ""
}

func (f fileset) CanCER() bool {
	return f.Ceron != "" && f.Ceroff != ""
}

func (f fileset) Empty() bool {
	return f.Rocon == "" && f.Rocoff == "" && f.Ceron == "" && f.Ceroff == ""
}

type Schedule struct {
	When    time.Time
	Entries []*Entry
}

func (s *Schedule) Create(w io.Writer, fs fileset) error {
	io.WriteString(os.Stdout, fmt.Sprintf("# %s-%s (build: %s)\n", Program, Version, BuildTime))
	io.WriteString(os.Stdout, fmt.Sprintf("# execution time: %s\n", ExecutionTime))
	io.WriteString(os.Stdout, fmt.Sprintf("# schedule start time: %s\n", s.When))
	io.WriteString(os.Stdout, "#\n")

	return nil
}

type Counter struct {
	Total   int
	Elapsed time.Duration
}

var (
	ExecutionTime time.Time
	DefaultBaseTime time.Time
)

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	ExecutionTime = time.Now().Truncate(time.Second).UTC()
	DefaultBaseTime = ExecutionTime.Add(Day).Truncate(Day).Add(time.Hour*10)
	log.Printf("%s-%s (build: %s) - %s", Program, Version, BuildTime, ExecutionTime.Format("2006-01-02 15:04:05"))
}

type command struct {
	File      string
	Delta     int
	Night     int
	Step      int
	Intersect int
}

const (
	PredictTimeIndex    = 0
	PredictEclipseIndex = 5
	PredictSaaIndex     = 6
	PredictColumns      = 8
	PredictComma        = ','
	PredictComment      = '#'
)

type predict struct {
	File       string
	Skip       bool
	Resolution int
	Time       int
	Eclipse    int
	Saa        int
	Trues      []string
	Falses     []string
}

func (p predict) Load() error {
	r, err := os.Open(p.File)
	if err != nil {
		return err
	}
	defer r.Close()

	rs := csv.NewReader(r)
	rs.Comment = PredictComment
	rs.Comma = PredictComma
	rs.FieldsPerRecord = PredictColumns

	if p.Skip {
		if _, err := rs.Read(); err != nil {
			return err
		}
	}
	for {
		r, err := rs.Read()
		if r == nil && err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ASIM semi automatic schedule generator tool\n")
		fmt.Fprintf(os.Stderr, "assist [-r] [-z] [-delta-rocon] [-delta-rocoff] [-delta-cer] [-i] <predict>\n")
		os.Exit(2)
	}
	var (
		d  delta
		fs fileset
	)
	flag.DurationVar(&d.Rocon, "delta-rocon", 10*time.Second, "delta ROC margin time (10s)")
	flag.DurationVar(&d.Rocoff, "delta-rocoff", 80*time.Second, "delta ROC margin time (80s)")
	flag.DurationVar(&d.Cer, "delta-cer", DefaultDeltaTime, "delta CER margin time (30s)")
	flag.DurationVar(&d.Intersect, "i", DefaultIntersectTime, "intersection time (2m)")
	flag.DurationVar(&d.AZM, "z", DefaultDeltaTime, "default AZM duration (30s)")
	flag.StringVar(&fs.Rocon, "rocon-file", "", "mxgs rocon command file")
	flag.StringVar(&fs.Rocoff, "rocoff-file", "", "mxgs rocoff command file")
	flag.StringVar(&fs.Ceron, "ceron-file", "", "mmia ceron command file")
	flag.StringVar(&fs.Ceroff, "ceroff-file", "", "mmia ceroff command file")
	flag.BoolVar(&fs.Keep, "keep-comment", false, "keep comment from command file")
	resolution := flag.Duration("r", time.Second*10, "prediction accuracy (10s)")
	baseTime := flag.String("base-time", DefaultBaseTime.Format("2006-01-02T15:04:05Z"), "schedule start time")
	flag.Parse()

	ts, err := listPeriods(flag.Arg(0), *resolution)
	if err != nil {
		log.Fatalln(err)
	}
	if ts == nil {
		log.Println("no records found in %s", flag.Arg(0))
		return
	}

	b, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil && *baseTime != "" {
		log.Fatalln(err)
	}
	if b.IsZero() {
		b = DefaultBaseTime
	}
	s, err := ts.Schedule(d, b)
	if err != nil {
		log.Fatalln(err)
	}
	if fs.Empty() {
		for i, e := range s.Entries {
			log.Printf("%3d | %7s | %s", i+1, e.Label, e.When.Truncate(time.Second).Format(timeFormat))
		}
		return
	}

	io.WriteString(os.Stdout, fmt.Sprintf("# %s-%s (build: %s)\n", Program, Version, BuildTime))
	io.WriteString(os.Stdout, fmt.Sprintf("# execution time: %s\n", ExecutionTime))
	io.WriteString(os.Stdout, fmt.Sprintf("# schedule start time: %s\n", s.When))
	io.WriteString(os.Stdout, "#\n")
	for _, f := range []string{flag.Arg(0), fs.Rocon, fs.Rocoff, fs.Ceron, fs.Ceroff} {
		if f == "" {
			continue
		}
		r, err := os.Open(f)
		if err != nil {
			log.Fatalln(err)
		}
		digest := md5.New()
		if _, err := io.Copy(digest, r); err != nil {
			log.Fatalln("file digest:", err)
		}
		s, err := r.Stat()
		if err != nil {
			log.Fatalln("file stat:", err)
		}
		mod := s.ModTime().Format("2006-02-01 15:04:05")
		row := fmt.Sprintf("# %s: md5 = %x, lastmod: %s, size (bytes): %d\n", f, digest.Sum(nil), mod, s.Size())
		io.WriteString(os.Stdout, row)
		r.Close()
		digest.Reset()
	}
	io.WriteString(os.Stdout, "#\n")

	cs := make(map[string]*Counter)
	es := make([]*Entry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e.When.Before(s.When) {
			continue
		}
		var (
			err    error
			offset time.Duration
		)
		delta := e.When.Sub(s.When)
		switch e.Label {
		case ROCON:
			if !fs.CanROC() {
				log.Fatalln("ROC files should be provided by pair (on/off)")
			}
			offset = d.Rocon
			delta, err = prepareCommand(fs.Rocon, e.When, delta, fs.Keep)
		case ROCOFF:
			if !fs.CanROC() {
				log.Fatalln("ROC files should be provided by pair (on/off)")
			}
			offset = d.Rocoff
			delta, err = prepareCommand(fs.Rocoff, e.When, delta, fs.Keep)
		case CERON:
			if !fs.CanCER() {
				log.Fatalln("CER files should be provided by pair (on/off)")
			}
			offset = d.Cer
			delta, err = prepareCommand(fs.Ceron, e.When, delta, fs.Keep)
		case CEROFF:
			if !fs.CanCER() {
				log.Fatalln("CER files should be provided by pair (on/off)")
			}
			offset = d.Cer
			delta, err = prepareCommand(fs.Ceroff, e.When, delta, fs.Keep)
		}
		if _, ok := cs[e.Label]; !ok {
			cs[e.Label] = &Counter{}
		}
		_, _ = offset, delta
		if err != nil {
			log.Fatalln(err)
		}
		cs[e.Label].Total++
		cs[e.Label].Elapsed += delta
		es = append(es, e)
	}
	if len(es) == 0 {
		return
	}
	first := es[0].When.Truncate(time.Second).Format("2006-01-02 15:04:05")
	last := es[len(es)-1].When.Truncate(time.Second).Format("2006-01-02 15:04:05")
	log.Printf("command(s) to be executed: %d (%s - %s)", len(es), first, last)
	for n, c := range cs {
		log.Printf("%d %s (%s)", c.Total, n, c.Elapsed)
	}
}

func prepareCommand(file string, w time.Time, delta time.Duration, keep bool) (time.Duration, error) {
	if file == "" {
		return 0, nil
	}
	r, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	s := bufio.NewScanner(r)
	// year := time.Date(w.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Add(DIFF+Leap)
	year := w.AddDate(0, 0, -w.YearDay()+1).Truncate(Day).Add(Leap)

	var elapsed time.Duration
	for s.Scan() {
		row := s.Text()
		if !strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("%d %s", int(delta.Seconds()), row)
			delta += Five
			elapsed += Five
			w = w.Add(Five)
		} else {
			// stamp := w.Add(DIFF+Leap).Truncate(step)
			stamp := w.Add(Leap).Truncate(Five)
			io.WriteString(os.Stdout, fmt.Sprintf("# SOY (GPS): %d/ GMT %3d/%s\n", stamp.Unix()-year.Unix(), stamp.YearDay(), stamp.Format("15:04:05")))
		}
		if keep || !strings.HasPrefix(row, "#") {
			io.WriteString(os.Stdout, row+"\n")
		}
	}
	return elapsed, s.Err()
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

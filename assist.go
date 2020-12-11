package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/midbel/toml"
)

type Assist struct {
	Alliop      string   `toml:"alliop"`
	Instr       string   `toml:"instrlist"`
	Trajectory  string   `toml:"path"`
	Resolution  Duration `toml:"resolution"`
	KeepComment bool     `toml:"keep-comment"`

	ROC    RocOption    `toml:"roc"`
	CER    CerOption    `toml:"cer"`
	ACS AuroraOption `toml:"acs"`

	*Schedule `toml:"-"`
}

func Default() *Assist {
	return &Assist{
		ROC:         rocDefault,
		CER:         cerDefault,
		ACS:      aurDefault,
		Instr:       INSTR,
		Alliop:      ALLIOP,
		KeepComment: true,
		Resolution:  NewDuration(1),
	}
}

func (a *Assist) Load(file string) error {
	if err := toml.DecodeFile(file, a); err != nil {
		return err
	}

	var (
		area = a.ACS.Area()
		err  error
	)
	if a.Trajectory != "" {
		a.Schedule, err = Open(a.Trajectory, a.Resolution.Duration, area)
	} else {
		a.Schedule, err = OpenReader(os.Stdin, a.Resolution.Duration, area)
	}
	return err
}

func (a *Assist) LoadAndFilter(file string, base time.Time) error {
	err := a.Load(file)
	if err == nil {
		a.Schedule = a.Schedule.Filter(base)
	}
	return err
}

func (a *Assist) Create() error {
	a.printSettings()
	var (
		w      io.Writer
		es     []Entry
		digest = md5.New()
	)
	switch f, err := os.Create(a.Alliop); {
	case err == nil:
		w = io.MultiWriter(f, digest)
		defer f.Close()
	case err != nil && a.Alliop == "":
		a.Alliop = "alliop"
		w = io.MultiWriter(digest, os.Stdout)
	default:
		return err
	}

	es, err := a.Schedule.Schedule(a.ROC, a.CER, a.ACS)
	if err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	a.printRanges(es)

	base := es[0].When.Add(-Five)
	a.writePreamble(w, base)
	if err := a.writeMetadata(w); err != nil {
		return err
	}

	ms, err := a.writeSchedule(w, es, base)
	if err != nil {
		return err
	}

	for n, c := range ms {
		log.Printf("%s scheduled: %d", n, c.Count)
	}

	var (
		rocdur = ms[ROCON].Duration + ms[ROCOFF].Duration
		cerdur = ms[CERON].Duration + ms[CEROFF].Duration
		acsdur = ms[ACSON].Duration + ms[ACSOFF].Duration
	)
	log.Printf("MXGS-ROC total time: %s", rocdur)
	log.Printf("MMIA-CER total time: %s", cerdur)
	log.Printf("ASIM-ACS total time: %s", acsdur)
	log.Printf("md5 %s: %x", a.Alliop, digest.Sum(nil))

	return a.writeList(rocdur > 0 || acsdur > 0, cerdur > 0)
}

func (a *Assist) PrintSettings() error {
  return nil
}

func (a *Assist) PrintPeriods() error {
	const (
		pattern = "%3d | %-8s | %s | %s | %s"
		timefmt = "2006-01-02T15:04:05"
	)
	var (
		nighttime, saatime, aurtime    time.Duration
		nightcount, saacount, aurcount int
	)
	for i, p := range a.Periods() {
		fmt.Printf(pattern, i, p.Label, p.Starts.Format(timefmt), p.Ends.Format(timefmt), p.Duration())
		fmt.Println()
		switch p.Label {
		case "saa":
			saatime += p.Duration()
			saacount++
		case "eclipse":
			nighttime += p.Duration()
			nightcount++
		case "aurora":
			aurtime += p.Duration()
			aurcount++
		}
	}
	fmt.Println()
	fmt.Printf("eclipse total time: %s (%d)", nighttime, nightcount)
	fmt.Println()
	fmt.Printf("saa total time: %s (%d)", saatime, saacount)
	fmt.Println()
	fmt.Printf("aurora total time: %s (%d)", aurtime, aurcount)
	fmt.Println()
	return nil
}

func (a *Assist) PrintEntries() error {
	const (
		hdrpat  = "%3s | %s | %-9s | %-9s | %-20s | %-20s"
		rowpat  = "%3d | %s | %-9s | %-9d | %-20s | %-20s"
		timefmt = "2006-01-02T15:04:05"
	)
	es, err := a.Schedule.Schedule(a.ROC, a.CER, a.ACS)
	if err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	first, last := es[0], es[len(es)-1]
	fmt.Printf(hdrpat, "#", "?", "TYPE", "SOY (GPS)", "START (GMT)", "END (GMT)")
	fmt.Println()
	fmt.Printf(rowpat, 0, " ", "SCHEDULE", SOY(first.When.Add(-Five)), first.When.Add(-Five).Format(timefmt), last.When.Format(timefmt))
	fmt.Println()

	var (
		roctime, certime, acstime    time.Duration
		roccount, cercount, acscount int
	)
	for i, e := range es {
		var to time.Time
		switch e.Label {
		case ROCON:
			to = e.When.Add(a.ROC.TimeOn.Duration)
			roctime += a.ROC.TimeOn.Duration
			roccount++
		case ROCOFF:
			to = e.When.Add(a.ROC.TimeOff.Duration)
			roctime += a.ROC.TimeOff.Duration
			roccount++
		case CERON:
			to = e.When.Add(a.ROC.TimeOn.Duration)
			certime += a.CER.TimeOn.Duration
			cercount++
		case CEROFF:
			to = e.When.Add(a.ROC.TimeOff.Duration)
			certime += a.CER.TimeOff.Duration
			cercount++
		case ACSON, ACSOFF:
			to = e.When.Add(a.ACS.Time.Duration)
			acstime += a.ACS.Time.Duration
			acscount++
		}
		conflict := "-"
		if e.Warning {
			conflict = "!"
		}

		fmt.Printf(rowpat, i+1, conflict, e.Label, e.SOY(), e.When.Format(timefmt), to.Format(timefmt))
		fmt.Println()
	}
	fmt.Printf("MXGS-ROC total time: %s (%d)", roctime, roccount)
	fmt.Println()
	fmt.Printf("MMIA-CER total time: %s (%d)", certime, cercount)
	fmt.Println()
	fmt.Printf("MXGS-ACS total time: %s (%d)", acstime, acscount)
	fmt.Println()
	return nil
}

type coze struct {
	Count    int
	Duration time.Duration
}

func (a *Assist) writeSchedule(w io.Writer, es []Entry, when time.Time) (map[string]coze, error) {
	var (
		err error
		cid = 1
		ms  = make(map[string]coze)
	)

	for _, e := range es {
		if e.When.Before(when) {
			continue
		}
		var (
			delta = e.When.Sub(when)
			curr  = ms[e.Label]
		)
		switch e.Label {
		case ROCON:
			if err := a.ROC.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.ROC.On, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.ROC.TimeOn.Duration
		case ROCOFF:
			if err := a.ROC.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.ROC.Off, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.ROC.TimeOff.Duration
		case CERON:
			if err := a.CER.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.CER.On, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.CER.TimeOn.Duration
		case CEROFF:
			if err := a.CER.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.CER.Off, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.CER.TimeOff.Duration
		case ACSON:
			if err := a.ACS.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.ACS.On, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.ACS.Time.Duration
		case ACSOFF:
			if err := a.ACS.Check(); err != nil {
				return nil, err
			}
			cid, delta, err = a.writeCommands(w, a.ACS.Off, cid, e.When, delta)
			curr.Count++
			curr.Duration += a.ACS.Time.Duration
		}
		if err != nil {
			return nil, err
		}
		ms[e.Label] = curr
	}
	return ms, nil
}

func (a *Assist) printSettings() {
	log.Printf("%s-%s (build: %s)", Program, Version, BuildTime)
	log.Printf("settings: AZM duration: %s", a.ROC.TimeAZM.Duration)
	log.Printf("settings: ROCON time: %s", a.ROC.TimeOn.Duration)
	log.Printf("settings: ROCOFF time: %s", a.ROC.TimeOff.Duration)
	log.Printf("settings: CER time: %s", a.CER.SwitchTime.Duration)
	log.Printf("settings: CERON time: %s", a.CER.TimeOn.Duration)
	log.Printf("settings: CEROFF time: %s", a.CER.TimeOff.Duration)
	log.Printf("settings: CER crossing duration: %s", a.CER.SaaCrossingTime.Duration)
	log.Printf("settings: ACS night duration: %s", a.ACS.Night.Duration)
	log.Printf("settings: ACS duration: %s", a.ACS.Time.Duration)
}

func (a *Assist) printRanges(es []Entry) {
	fst, lst := es[0], es[len(es)-1]
	log.Printf("first command (%s) at %s (%d)", fst.Label, fst.When.Format(timeFormat), SOY(fst.When))
	log.Printf("last command (%s) at %s (%d)", lst.Label, lst.When.Format(timeFormat), SOY(lst.When))
}

func (a *Assist) writePreamble(w io.Writer, when time.Time) {
	var (
		year  = when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day).Add(Leap)
		stamp = when.Add(Leap)
	)

	fmt.Fprintf(w, "# %s-%s (build: %s)", Program, Version, BuildTime)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# "+strings.Join(os.Args, " "))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "# execution time: %s", ExecutionTime)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "# schedule start time: %s (SOY: %d)", when, (stamp.Unix()-year.Unix())+int64(Leap.Seconds()))
	fmt.Fprintln(w)
	fmt.Fprintln(w)
}

func (a *Assist) writeMetadata(w io.Writer) error {
	aboutFile := func(file string, digest hash.Hash) error {
		defer digest.Reset()

		r, err := os.Open(file)
		if err != nil {
			return checkError(err, nil)
		}
		defer r.Close()

		if _, err := io.Copy(digest, r); err != nil {
			return checkError(err, nil)
		}
		s, err := r.Stat()
		if err != nil {
			return checkError(err, nil)
		}
		var (
			modtime  = s.ModTime().Format("2006-01-02 15:04:05")
			filesize = s.Size()
			sum      = digest.Sum(nil)
		)
		log.Printf("%s: md5 = %x, lastmod: %s, size: %d bytes", file, sum, modtime, filesize)
		fmt.Fprintf(w, "# %s: md5 = %x, lastmod: %s, size : %d bytes", file, sum, modtime, filesize)
		fmt.Fprintln(w)
		return nil
	}
	var (
		files = []string{
			a.Trajectory,
			a.ROC.On,
			a.ROC.Off,
			a.CER.On,
			a.CER.Off,
			a.ACS.On,
			a.ACS.Off,
		}
		digest = md5.New()
	)
	for _, f := range files {
		if f == "" {
			continue
		}
		if err := aboutFile(f, digest); err != nil {
			return err
		}
	}
	fmt.Fprintln(w)
	return nil
}

const (
	InstrMMIA = "MMIA 129"
	InstrMXGS = "MXGS 128"
)

func (a *Assist) writeList(mxgs, mmia bool) error {
	switch f, err := os.Create(a.Instr); {
	case err == nil:
		defer f.Close()

		var (
			digest = md5.New()
			w      = io.MultiWriter(f, digest)
		)

		if mxgs {
			fmt.Fprintln(w, InstrMXGS)
		}
		if mmia {
			fmt.Fprintln(w, InstrMMIA)
		}
		log.Printf("md5 %s: %x", a.Instr, digest.Sum(nil))
	case err != nil && a.Instr == "":
	default:
		return checkError(err, nil)
	}
	return nil
}

func (a *Assist) writeCommands(w io.Writer, file string, cid int, when time.Time, delta time.Duration) (int, time.Duration, error) {
	if file == "" {
		return cid, 0, nil
	}
	bs, err := ioutil.ReadFile(file)
	if err != nil {
		return cid, 0, checkError(err, nil)
	}
	d := scheduleDuration(bytes.NewReader(bs))
	if d <= 0 {
		return cid, 0, nil
	}

	s := bufio.NewScanner(bytes.NewReader(bs))
	year := when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day)

	var elapsed time.Duration
	if a.KeepComment {
		fmt.Fprintf(w, "# %s: %s (execution time: %s)", file, when.Format(timeFormat), d)
		fmt.Fprintln(w)
	}
	for s.Scan() {
		row := s.Text()
		if !strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("%d %s", int(delta.Seconds()), row)
			delta += Five
			elapsed += Five
			when = when.Add(Five)
		} else {
			stamp := when //.Truncate(Five)
			soy := (stamp.Unix() - year.Unix()) + int64(Leap.Seconds())
			fmt.Fprintf(w, "# SOY (GPS): %d/ GMT %03d/%s", soy, stamp.YearDay(), stamp.Format("15:04:05"))
			fmt.Fprintln(w)
		}
		if a.KeepComment && strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("# CMD %d: %s", cid, strings.TrimPrefix(row, "#"))
			cid++
		}
		if a.KeepComment || !strings.HasPrefix(row, "#") {
			fmt.Fprintln(w, row)
		}
	}
	switch e := s.Err(); e {
	case bufio.ErrTooLong, bufio.ErrNegativeAdvance, bufio.ErrAdvanceTooFar:
		err = badUsage(fmt.Sprintf("%s: processing failed (%v)", file, e))
	default:
		if e != nil {
			err = badUsage(err.Error())
		}
	}
	fmt.Fprintln(w)
	return cid, elapsed, err
}

func scheduleDuration(r io.Reader) time.Duration {
	s := bufio.NewScanner(r)

	var d time.Duration
	for s.Scan() {
		if t := s.Text(); !strings.HasPrefix(t, "#") {
			d += Five
		}
	}
	return d
}

package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/midbel/toml"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const Leap = 18 * time.Second

const (
	Version   = "0.1.0-RC1"
	BuildTime = "2018-11-14 08:11:00"
	Program   = "assist"
)

const (
	DefaultDeltaTime     = time.Second * 30
	DefaultIntersectTime = time.Second * 120
	Day                  = time.Hour * 24
	Five                 = time.Second * 5
)

const (
	ROCON  = "ROCON"
	ROCOFF = "ROCOFF"
	CERON  = "CERON"
	CEROFF = "CEROFF"
)

const (
	ALLIOP = "alliop"
	INSTR  = "instrlist"
)

const (
	InstrMMIA = "MMIA 129"
	InstrMXGS = "MXGS 128"
)

var (
	ExecutionTime   time.Time
	DefaultBaseTime time.Time
)

var LineBreak string

type delta struct {
	Rocon     time.Duration
	Rocoff    time.Duration
	Cer       time.Duration
	Wait      time.Duration
	Intersect time.Duration
	AZM       time.Duration
}

type fileset struct {
	Rocon       string
	Rocoff      string
	Ceron       string
	Ceroff      string
	Keep        bool
	WithoutList bool
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

func init() {
	ExecutionTime = time.Now().Truncate(time.Second).UTC()
	DefaultBaseTime = ExecutionTime.Add(Day).Truncate(Day).Add(time.Hour * 10)

	log.SetOutput(os.Stderr)
	log.SetFlags(0)
}

type Duration struct {
	time.Duration
}

func (d *Duration) Set(v string) error {
	i, err := time.ParseDuration(v)
	if err == nil {
		d.Duration = i
	}
	return err
}

type command struct {
	File     string   `toml:"file"`
	AZM      Duration `toml:"azm"`
	Time     Duration `toml:"time"`
	Night    Duration `toml:"wait"`
	Crossing Duration `toml:"intersection"`
	Step     int      `toml:"step"`
}

type trajectory struct {
	File    string   `toml:"time"`
	Step    Duration `toml:"resolution"`
	Time    int      `toml:"time"`
	Eclipse int      `toml:"eclipse"`
	Saa     int      `toml:"saa"`
}

type cerroc struct {
	Alliop string      `toml:"alliop"`
	Instr  string      `toml:"instrlist"`
	Keep   bool        `toml:"keep-comment"`
	Ceron  *command    `toml:"ceron"`
	Ceroff *command    `toml:"ceroff"`
	Rocon  *command    `toml:"rocon"`
	Rocoff *command    `toml:"rocoff"`
	Path   *trajectory `toml:"trajectory"`
}

func main() {
	var (
		d  delta
		fs fileset
	)
	flag.DurationVar(&d.Rocon, "rocon-time", 50*time.Second, "ROCON execution time")
	flag.DurationVar(&d.Rocoff, "rocoff-time", 80*time.Second, "ROCOFF execution time")
	flag.DurationVar(&d.Wait, "rocon-wait", 90*time.Second, "wait time before starting ROCON")
	flag.DurationVar(&d.Cer, "cer-time", 300*time.Second, "delta CER margin time")
	flag.DurationVar(&d.Intersect, "cer-crossing", DefaultIntersectTime, "intersection time to enable CER")
	flag.DurationVar(&d.AZM, "azm", 40*time.Second, "default AZM duration")
	flag.StringVar(&fs.Rocon, "rocon-file", "", "mxgs rocon command file")
	flag.StringVar(&fs.Rocoff, "rocoff-file", "", "mxgs rocoff command file")
	flag.StringVar(&fs.Ceron, "ceron-file", "", "mmia ceron command file")
	flag.StringVar(&fs.Ceroff, "ceroff-file", "", "mmia ceroff command file")
	flag.BoolVar(&fs.Keep, "keep-comment", false, "keep comment from command file")
	flag.BoolVar(&fs.WithoutList, "no-instr-list", false, "do not create instrument list")
	datadir := flag.String("datadir", "", "write alliop and instrlist to directory")
	baseTime := flag.String("base-time", DefaultBaseTime.Format("2006-01-02T15:04:05Z"), "schedule start time")
	resolution := flag.Duration("resolution", time.Second*10, "prediction accuracy")
	config := flag.Bool("config", false, "use configuration file")
	list := flag.Bool("list", false, "schedule list")
	flag.Parse()

	b, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil && *baseTime != "" {
		log.Fatalln(err)
	}
	if b.IsZero() {
		b = DefaultBaseTime
	}
	var s *Schedule
	if *config {
		s, err = loadScheduleFromConfig(flag.Arg(0))
	} else {
		switch flag.NArg() {
		default:
			s, err = Open(flag.Arg(0), *resolution)
		case 0:
			s, err = OpenReader(os.Stdin, *resolution)
		}
	}
	if err != nil {
		log.Fatalln(err)
	}
	if *list {
		es, err := s.Schedule(d, true, true)
		if err != nil {
			log.Fatalln(err)
		}
		log.SetOutput(os.Stdout)
		for i, e := range es {
			log.Printf("%3d | %7s | %s", i+1, e.Label, e.When.Truncate(time.Second).Format(timeFormat))
		}
		return
	}
	if fs.Empty() {
		log.Fatalln("no command files provided")
	}
	s = s.Filter(b)
	var w io.Writer = os.Stdout
	if i, err := os.Stat(*datadir); err == nil && i.IsDir() {
		f, err := os.Create(filepath.Join(*datadir, ALLIOP))
		if err != nil {
			log.Fatalln(err)
		}
		defer f.Close()
		if !fs.WithoutList {
			f, err := os.Create(filepath.Join(*datadir, INSTR))
			if err != nil {
				log.Fatalln(err)
			}
			if fs.CanROC() {
				io.WriteString(f, InstrMXGS+LineBreak)
			}
			if fs.CanCER() {
				io.WriteString(f, InstrMMIA+LineBreak)
			}
			f.Close()
		}
		w = f
	}
	es, err := s.Schedule(d, fs.CanROC(), fs.CanCER())
	if err != nil {
		log.Fatalln(err)
	}
	if len(es) == 0 {
		return
	}
	b = es[0].When.Add(-time.Second * 5)
	writePreamble(w, b)
	if err := writeMetadata(w, flag.Arg(0), fs); err != nil {
		log.Fatalln(err)
	}
	if err := writeSchedule(w, es, b, fs); err != nil {
		log.Fatalln(err)
	}
}

func loadScheduleFromConfig(file string) (*Schedule, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	c := struct{}{}
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("not yet implemented")
}

func writeSchedule(w io.Writer, es []*Entry, when time.Time, fs fileset) error {
	cid := 1
	var err error
	for _, e := range es {
		if e.When.Before(when) {
			continue
		}
		delta := e.When.Sub(when)
		switch e.Label {
		case ROCON:
			if !fs.CanROC() {
				return missingFile("ROC")
			}
			cid, delta, err = prepareCommand(w, fs.Rocon, cid, e.When, delta, fs.Keep)
		case ROCOFF:
			if !fs.CanROC() {
				return missingFile("ROC")
			}
			cid, delta, err = prepareCommand(w, fs.Rocoff, cid, e.When, delta, fs.Keep)
		case CERON:
			if !fs.CanCER() {
				return missingFile("CER")
			}
			cid, delta, err = prepareCommand(w, fs.Ceron, cid, e.When, delta, fs.Keep)
		case CEROFF:
			if !fs.CanCER() {
				return missingFile("CER")
			}
			cid, delta, err = prepareCommand(w, fs.Ceroff, cid, e.When, delta, fs.Keep)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writePreamble(w io.Writer, when time.Time) {
	year := when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day).Add(Leap)
	stamp := when.Add(Leap).Truncate(Five)

	io.WriteString(w, fmt.Sprintf("# %s-%s (build: %s)", Program, Version, BuildTime))
	io.WriteString(w, LineBreak)
	io.WriteString(w, fmt.Sprintf("# execution time: %s", ExecutionTime))
	io.WriteString(w, LineBreak)
	io.WriteString(w, fmt.Sprintf("# schedule start time: %s (SOY: %d)", when, stamp.Unix()-year.Unix()))
	io.WriteString(w, LineBreak)
	io.WriteString(w, "#"+LineBreak)
}

func writeMetadata(w io.Writer, rs string, fs fileset) error {
	for _, f := range []string{rs, fs.Rocon, fs.Rocoff, fs.Ceron, fs.Ceroff} {
		if f == "" {
			continue
		}
		r, err := os.Open(f)
		if err != nil {
			return err
		}
		defer r.Close()

		digest := md5.New()
		if _, err := io.Copy(digest, r); err != nil {
			return err
		}
		s, err := r.Stat()
		if err != nil {
			return err
		}
		mod := s.ModTime().Format("2006-02-01 15:04:05")
		row := fmt.Sprintf("# %s: md5 = %x, lastmod: %s, size (bytes): %d", f, digest.Sum(nil), mod, s.Size())
		io.WriteString(w, row)
		io.WriteString(w, LineBreak)
		digest.Reset()
	}
	io.WriteString(w, "#")
	io.WriteString(w, LineBreak)
	return nil
}

func prepareCommand(w io.Writer, file string, cid int, when time.Time, delta time.Duration, keep bool) (int, time.Duration, error) {
	if file == "" {
		return cid, 0, nil
	}
	bs, err := ioutil.ReadFile(file)
	if err != nil {
		return cid, 0, err
	}
	d := scheduleDuration(bytes.NewReader(bs))
	if d <= 0 {
		return cid, 0, nil
	}

	s := bufio.NewScanner(bytes.NewReader(bs))
	// year := time.Date(w.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Add(DIFF+Leap)
	year := when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day).Add(Leap)

	var elapsed time.Duration
	if keep {
		io.WriteString(w, "#")
		io.WriteString(w, LineBreak)
		io.WriteString(w, fmt.Sprintf("# %s: %s (execution time: %s)", file, when.Format(timeFormat), d))
		io.WriteString(w, LineBreak)
	}
	for s.Scan() {
		row := s.Text()
		if !strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("%d %s", int(delta.Seconds()), row)
			delta += Five
			elapsed += Five
			when = when.Add(Five)
		} else {
			// stamp := w.Add(DIFF+Leap).Truncate(step)
			stamp := when.Add(Leap).Truncate(Five)
			io.WriteString(w, fmt.Sprintf("# SOY (GPS): %d/ GMT %3d/%s", stamp.Unix()-year.Unix(), stamp.YearDay(), stamp.Format("15:04:05")))
			io.WriteString(w, LineBreak)
		}
		if keep && strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("# CMD %d: %s", cid, strings.TrimPrefix(row, "#"))
			cid++
		}
		if keep || !strings.HasPrefix(row, "#") {
			io.WriteString(w, row)
			io.WriteString(w, LineBreak)
		}
	}
	return cid, elapsed, s.Err()
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

func missingFile(n string) error {
	return fmt.Errorf("%s files should be provided by pair (on/off)", strings.ToUpper(n))
}

package main

import (
	"bufio"
	"crypto/md5"
	"flag"
	"io"
	"fmt"
	"log"
	"os"
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
	Day                  = time.Hour * 24
	Five                 = time.Second * 5
)

const (
	ROCON  = "ROCON"
	ROCOFF = "ROCOFF"
	CERON  = "CERON"
	CEROFF = "CEROFF"
)

var (
	ExecutionTime   time.Time
	DefaultBaseTime time.Time
)

type delta struct {
	Rocon     time.Duration
	Rocoff    time.Duration
	Cer       time.Duration
	Wait      time.Duration
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

func init() {
	ExecutionTime = time.Now().Truncate(time.Second).UTC()
	DefaultBaseTime = ExecutionTime.Add(Day).Truncate(Day).Add(time.Hour * 10)

	log.SetOutput(os.Stdout)
	log.SetFlags(0)
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
	flag.DurationVar(&d.Rocon, "rocon-delta", 50*time.Second, "delta ROC margin time (10s)")
	flag.DurationVar(&d.Rocoff, "rocoff-delta", 80*time.Second, "delta ROC margin time (80s)")
	flag.DurationVar(&d.Wait, "rocon-wait", 90*time.Second, "wait time before starting ROC (90s)")
	flag.DurationVar(&d.Cer, "cer-delta", DefaultDeltaTime, "delta CER margin time (30s)")
	flag.DurationVar(&d.Intersect, "i", DefaultIntersectTime, "intersection time (120s)")
	flag.DurationVar(&d.AZM, "z", DefaultDeltaTime, "default AZM duration (30s)")
	flag.StringVar(&fs.Rocon, "rocon-file", "", "mxgs rocon command file")
	flag.StringVar(&fs.Rocoff, "rocoff-file", "", "mxgs rocoff command file")
	flag.StringVar(&fs.Ceron, "ceron-file", "", "mmia ceron command file")
	flag.StringVar(&fs.Ceroff, "ceroff-file", "", "mmia ceroff command file")
	flag.BoolVar(&fs.Keep, "keep-comment", false, "keep comment from command file")
	baseTime := flag.String("base-time", DefaultBaseTime.Format("2006-01-02T15:04:05Z"), "schedule start time")
	resolution := flag.Duration("r", time.Second*10, "prediction accuracy (10s)")
	flag.Parse()

	ts, err := listPeriods(flag.Arg(0), *resolution)
	if err != nil {
		log.Fatalln(err)
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
	}
	writePreamble(os.Stdout, s.When)
	if err := writeMetadata(os.Stdout, flag.Arg(0), fs); err != nil {
		log.Fatalln(err)
	}
	if err := writeSchedule(os.Stdout, s, fs); err != nil {
		log.Fatalln(err)
	}
}

func writeSchedule(w io.Writer, s *Schedule, fs fileset) error {
	var err error
	for _, e := range s.Entries {
		if e.When.Before(s.When) {
			continue
		}
		delta := e.When.Sub(s.When)
		switch e.Label {
		case ROCON:
			if !fs.CanROC() {
				return missingFile("ROC")
			}
			delta, err = prepareCommand(fs.Rocon, e.When, delta, fs.Keep)
		case ROCOFF:
			if !fs.CanROC() {
				return missingFile("ROC")
			}
			delta, err = prepareCommand(fs.Rocoff, e.When, delta, fs.Keep)
		case CERON:
			if !fs.CanCER() {
				return missingFile("CER")
			}
			delta, err = prepareCommand(fs.Ceron, e.When, delta, fs.Keep)
		case CEROFF:
			if !fs.CanCER() {
				return missingFile("CER")
			}
			delta, err = prepareCommand(fs.Ceroff, e.When, delta, fs.Keep)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writePreamble(w io.Writer, when time.Time) {
	io.WriteString(w, fmt.Sprintf("# %s-%s (build: %s)\n", Program, Version, BuildTime))
	io.WriteString(w, fmt.Sprintf("# execution time: %s\n", ExecutionTime))
	io.WriteString(w, fmt.Sprintf("# schedule start time: %s\n", when))
	io.WriteString(w, "#\n")
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
		row := fmt.Sprintf("# %s: md5 = %x, lastmod: %s, size (bytes): %d\n", f, digest.Sum(nil), mod, s.Size())
		io.WriteString(w, row)
		digest.Reset()
	}
	io.WriteString(w, "#\n")
	return nil
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

func missingFile(n string) error {
	return fmt.Errorf("%s files should be provided by pair (on/off)", strings.ToUpper(n))
}

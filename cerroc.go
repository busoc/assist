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
	// "path/filepath"
	"strings"
	"time"

	"github.com/midbel/toml"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const (
	Version   = "0.1.0-beta"
	BuildTime = "2018-11-26 06:35:00"
	Program   = "assist"
)

const (
	ROCON  = "ROCON"
	ROCOFF = "ROCOFF"
	CERON  = "CERON"
	CEROFF = "CEROFF"
)

const (
	ALLIOP = "alliop.txt"
	INSTR  = "instrlist.txt"
)

const (
	InstrMMIA = "MMIA 129"
	InstrMXGS = "MXGS 128"
)

var (
	ExecutionTime   time.Time
	DefaultBaseTime time.Time
)

type delta struct {
	Rocon     Duration `toml:"rocon"`
	Rocoff    Duration `toml:"rocoff"`
	Cer       Duration `toml:"cer"`
	Wait      Duration `toml:"wait"`
	Intersect Duration `toml:"crossing"`
	AZM       Duration `toml:"azm"`
}

type fileset struct {
	Rocon       string `toml:"rocon"`
	Rocoff      string `toml:"rocoff"`
	Ceron       string `toml:"ceron"`
	Ceroff      string `toml:"ceroff"`
	Keep        bool   `toml:"-"`

	Alliop    string `toml:"-"`
	Instrlist string `toml:"-"`
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

const helpText = `ASIM Semi Automatic Schedule Tool

Usage: assist [options] <trajectory.csv>

Command files:

assist accepts command files by pair. In other words, if the ROCON file is given,
the ROCOFF should also be provided. The same is true for the CERON/CEROFF files.

However, it is not mandatory to have the 4 files provided. A schedule can be
created only for ROC or for CER (see examples below).

It is an error to not provide any file unless if the list flag is given to assist.

Input format:

the input of assist consists of a tabulated "file". The columns of the file are:

- datetime (YYYY-mm-dd HH:MM:SS.ssssss)
- modified julian day
- altitude (kilometer)
- latitude (degree or DMS)
- longitude (degree or DMS)
- eclipse (1: night, 0: day)
- crossing (1: crossing, 0: no crossing)
- TLE epoch

assist only uses the columns from the input file (but all are mandatory even if
empty):

- datetime
- eclipse
- crossing

the values accepted by assist to decide if the trajectory is "entering" SAA/
Eclipse, are: 1, on, true

the values accepted by assist to decide if the trajectory is "leaving" SAA/
Eclipse are: 0, off, false

Options:

  -rocon-time     TIME  ROCON expected execution time
  -rocoff-time    TIME  ROCOFF expected execution time
  -rocon-wait     TIME  wait TIME after entering Eclipse before starting ROCON
  -cer-time       TIME  TIME before Eclipse to switch CER(ON|OFF)
  -cer-crossing   TIME  minimum crossing time of SAA and Eclipse to switch CER(ON|OFF)
  -azm            TIME  AZM duration
  -rocon-file     FILE  use FILE with commands for ROCON
  -rocoff-file    FILE  use FILE with commands for ROCOFF
  -ceron-file     FILE  use FILE with commands for CERON
  -ceroff-file    FILE  use FILE with commands for CEROFF
  -resolution     TIME  TIME interval between two rows in the trajectory
  -base-time      DATE
  -alliop         FILE  save schedule to FILE
  -instrlist      FILE  save instrlist to FILE
  -keep-comment         keep comment (if any) from command files
  -list                 print the list of commands instead of creating a schedule
  -config               load settings from a configuration file

Examples:

# create a full schedule keeping the comments from the given command files with
a longer AZM for ROCON/ROCOFF and a larger margin for CERON/CEROFF
$ assist -keep-comment -base-time 2018-11-19T12:35:00Z \
  -rocon-file /usr/local/etc/asim/MXGS-ROCON.txt \
  -rocoff-file /usr/local/etc/asim/MXGS-ROCOFF.txt \
  -ceron-file /usr/local/etc/asim/MMIA-CERON.txt \
  -ceroff-file /usr/local/etc/asim/MMIA-CEROFF.txt \
  -azm 80s \
  -cer-time 900s \
  -datadir /var/asim/2018-310/ \
  inspect-trajectory.csv

# create a schedule only for CER (the same can be done for ROC).
$ assist -keep-comment -base-time 2018-11-19T12:35:00Z \
  -ceron-file /usr/local/etc/asim/MMIA-CERON.txt \
  -ceroff-file /usr/local/etc/asim/MMIA-CEROFF.txt \
  -cer-time 900s \
  -datadir /var/asim/CER-2018-310/ \
  inspect-trajectory.csv

# print the list of commands that could be scheduled from a local file
$ assist -list tmp/inspect-trajectory.csv

# print the list of commands that could be scheduled from the output of inspect
$ inspect -d 24h -i 10s -f csv /tmp/tle-2018305.txt | assist -list

# use a configuration file instead of command line options
$ assist -config /usr/local/etc/asim/cerroc-ops.toml
`

func init() {
	ExecutionTime = time.Now().Truncate(time.Second).UTC()
	DefaultBaseTime = ExecutionTime.Add(Day).Truncate(Day).Add(time.Hour * 10)

	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, helpText)
		os.Exit(2)
	}
}

func main() {
	var fs fileset
	d := delta {
		Rocon: Duration{time.Second*50},
		Rocoff: Duration{time.Second*80},
		Cer: Duration{time.Second*300},
		Intersect: Duration{DefaultIntersectTime},
		AZM: Duration{time.Second*40},
	}
	flag.Var(&d.Rocon, "rocon-time", "ROCON execution time")
	flag.Var(&d.Rocoff, "rocoff-time", "ROCOFF execution time")
	flag.Var(&d.Wait, "rocon-wait", "wait time before starting ROCON")
	flag.Var(&d.Cer, "cer-time", "delta CER margin time")
	flag.Var(&d.Intersect, "cer-crossing", "intersection time to enable CER")
	flag.Var(&d.AZM, "azm", "default AZM duration")
	flag.StringVar(&fs.Rocon, "rocon-file", "", "mxgs rocon command file")
	flag.StringVar(&fs.Rocoff, "rocoff-file", "", "mxgs rocoff command file")
	flag.StringVar(&fs.Ceron, "ceron-file", "", "mmia ceron command file")
	flag.StringVar(&fs.Ceroff, "ceroff-file", "", "mmia ceroff command file")
	flag.BoolVar(&fs.Keep, "keep-comment", false, "keep comment from command file")
	flag.StringVar(&fs.Alliop, "alliop", "", "alliop file")
	flag.StringVar(&fs.Instrlist, "instrlist", "", "instrlist file")
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
		s, err = loadFromConfig(flag.Arg(0), &d, &fs)
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
		if !b.Equal(DefaultBaseTime) {
			s = s.Filter(b)
		}
		es, err := s.Schedule(d, true, true)
		if err != nil {
			log.Fatalln(err)
		}
		log.SetOutput(os.Stdout)
		for i, e := range es {
			log.Printf("%3d | %-7s | %s | %d", i+1, e.Label, e.When.Format("2006-01-02T15:04:05"), e.SOY())
		}
		return
	}
	if fs.Empty() {
		log.Fatalln("no command files provided")
	}
	var w io.Writer = os.Stdout
	switch f, err := os.Create(fs.Alliop); {
	case err == nil:
		w = f
		defer f.Close()
	case err != nil && fs.Alliop == "":
	default:
		log.Fatalln(err)
	}
	switch f, err := os.Create(fs.Instrlist); {
	case err == nil:
		defer f.Close()
		if fs.CanROC() {
			fmt.Fprintln(f, InstrMXGS)
		}
		if fs.CanCER() {
			fmt.Fprintln(f, InstrMMIA)
		}
	case err != nil && fs.Instrlist == "":
	default:
		log.Fatalln(err)
	}
	es, err := s.Filter(b).Schedule(d, fs.CanROC(), fs.CanCER())
	if err != nil {
		log.Fatalln(err)
	}
	if len(es) == 0 {
		return
	}
	b = es[0].When.Add(-Five)
	writePreamble(w, b)
	if err := writeMetadata(w, flag.Arg(0), fs); err != nil {
		log.Fatalln(err)
	}
	if err := writeSchedule(w, es, b, fs); err != nil {
		log.Fatalln(err)
	}
}

type Duration struct {
	time.Duration
}

func (d *Duration) String() string {
	return d.Duration.String()
}

func (d *Duration) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err == nil {
		d.Duration = v
	}
	return err
}

func loadFromConfig(file string, d *delta, fs *fileset) (*Schedule, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	c := struct{
		Resolution Duration `toml:"resolution"`
		Trajectory string `toml:"path"`

		Alliop string `toml:"alliop"`
		Instr  string `toml:"instrlist"`
		Comment bool `toml:"keep-comment"`

		Delta *delta `toml:"delta"`
		Commands *fileset `toml:"commands"`
	}{
		Delta: d,
		Commands: fs,
	}
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return nil, err
	}
	fs.Alliop, fs.Instrlist, fs.Keep = c.Alliop, c.Instr, c.Comment
	return Open(c.Trajectory, c.Resolution.Duration)
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
	stamp := when.Add(Leap)

	fmt.Fprintf(w, "# %s-%s (build: %s)", Program, Version, BuildTime)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "# execution time: %s", ExecutionTime)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "# schedule start time: %s (SOY: %d)", when, (stamp.Unix()-year.Unix())+int64(Leap.Seconds()))
	fmt.Fprintln(w)
	fmt.Fprintln(w)
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
		mod := s.ModTime().Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "# %s: md5 = %x, lastmod: %s, size (bytes): %d", f, digest.Sum(nil), mod, s.Size())
		fmt.Fprintln(w)
		digest.Reset()
	}
	fmt.Fprintln(w)
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
	year := when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day)

	var elapsed time.Duration
	if keep {
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
		if keep && strings.HasPrefix(row, "#") {
			row = fmt.Sprintf("# CMD %d: %s", cid, strings.TrimPrefix(row, "#"))
			cid++
		}
		if keep || !strings.HasPrefix(row, "#") {
			fmt.Fprintln(w, row)
		}
	}
	fmt.Fprintln(w)
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

package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/midbel/toml"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const (
	Version   = "1.2.0"
	BuildTime = "2020-12-08 10:45:00"
	Program   = "assist"
)

func init() {
	ExecutionTime = time.Now().Truncate(time.Second).UTC()
	DefaultBaseTime = ExecutionTime.Add(Day).Truncate(Day).Add(time.Hour * 10)

	log.SetOutput(os.Stderr)
	log.SetPrefix(fmt.Sprintf("[%s-%s] ", Program, Version))

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, helpText)
		os.Exit(2)
	}
}

func main() {
	var (
		fs       fileset
		d        = DefaultDelta
		baseTime = flag.String("base-time", DefaultBaseTime.Format("2006-01-02T15:04:05Z"), "schedule start time")
		elist    = flag.Bool("list-entries", false, "schedule list")
		plist    = flag.Bool("list-periods", false, "periods list")
		version  = flag.Bool("version", false, "print version and exists")
	)
	flag.Parse()

	if *version {
		fmt.Fprintf(os.Stderr, "%s-%s (%s)\n", Program, Version, BuildTime)
		return
	}

	base, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil && *baseTime != "" {
		Exit(badUsage("base-time format invalid"))
	}
	if base.IsZero() {
		base = DefaultBaseTime
	}
	s, err := loadFromConfig(flag.Arg(0), &d, &fs)
	if err != nil {
		Exit(err)
	}
	s = s.Filter(base)
	if *plist {
		ListPeriods(s)
		return
	}
	if *elist {
		if err := ListEntries(s, d, fs, false); err != nil {
			Exit(err)
		}
		return
	}
	err = createSchedule(s, d, fs)
	Exit(checkError(err, nil))
}

func createSchedule(s *Schedule, d delta, fs fileset) error {
	if err := fs.Can(); err != nil {
		return err
	}
	dumpSettings(d, fs)

	var (
		w      io.Writer
		es     []*Entry
		digest = md5.New()
	)
	switch f, err := os.Create(fs.Alliop); {
	case err == nil:
		w = io.MultiWriter(f, digest)
		defer f.Close()
	case err != nil && fs.Alliop == "":
		fs.Alliop = "alliop"
		w = io.MultiWriter(digest, os.Stdout)
	default:
		return err
	}
	es, err := s.Schedule(d, fs.CanROC(), fs.CanCER(), fs.CanACS())
	if err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	first, last := es[0], es[len(es)-1]
	log.Printf("first command (%s) at %s (%d)", first.Label, first.When.Format(timeFormat), SOY(first.When))
	log.Printf("last command (%s) at %s (%d)", last.Label, last.When.Format(timeFormat), SOY(last.When))

	base := es[0].When.Add(-Five)
	writePreamble(w, base)
	if err := writeMetadata(w, fs); err != nil {
		return err
	}
	ms, err := writeSchedule(w, es, base, fs)
	if err != nil {
		return err
	}

	for n, c := range ms {
		log.Printf("%s scheduled: %d", n, c)
	}

	_, tr := TimeROC(es, d)
	log.Printf("MXGS-ROC total time: %s", tr)
	_, tc := TimeCER(es, d)
	log.Printf("MMIA-CER total time: %s", tc)
	_, ta := TimeACS(es, d)
	log.Printf("ASIM-ACS total time: %s", ta)
	log.Printf("md5 %s: %x", fs.Alliop, digest.Sum(nil))

	return writeList(fs.Instrlist, fs.CanROC() && tr > 0, fs.CanCER() && tc > 0)
}

func loadFromConfig(file string, d *delta, fs *fileset) (*Schedule, error) {
	c := struct {
		Resolution Duration `toml:"resolution"`
		Trajectory string   `toml:"path"`

		Alliop  string `toml:"alliop"`
		Instr   string `toml:"instrlist"`
		Comment bool   `toml:"keep-comment"`

		Area struct {
			Boxes []Rect `toml:"boxes"`
		} `toml:"area"`

		Delta    *delta   `toml:"delta"`
		Commands *fileset `toml:"commands"`
	}{
		Delta:    d,
		Commands: fs,
	}
	if err := toml.DecodeFile(file, &c); err != nil {
		return nil, badUsage(fmt.Sprintf("invalid configuration file: %v", err))
	}
	fs.Path, fs.Alliop, fs.Instrlist, fs.Keep = c.Trajectory, c.Alliop, c.Instr, c.Comment

	cs := make([]Shape, len(c.Area.Boxes))
	for i := range c.Area.Boxes {
		cs[i] = c.Area.Boxes[i]
	}
	area := NewArea(cs...)
	if c.Trajectory != "" {
		return Open(c.Trajectory, c.Resolution.Duration, area)
	}
	return OpenReader(os.Stdin, c.Resolution.Duration, area)
}

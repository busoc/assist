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
	Version   = "1.1.1"
	BuildTime = "2019-01-30 10:05:00"
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
	var fs fileset
	d := delta{
		Rocon:  Duration{time.Second * 50},
		Rocoff: Duration{time.Second * 80},
		Ceron:  Duration{time.Second * 40},
		Ceroff: Duration{time.Second * 40},
		Margin: Duration{time.Second * 120},
		// Cer:          Duration{time.Second * 300},
		Cer:          Duration{0},
		Intersect:    Duration{DefaultIntersectTime},
		AZM:          Duration{time.Second * 40},
		Saa:          Duration{time.Second * 10},
		CerBefore:    Duration{time.Second * 50},
		CerAfter:     Duration{time.Second * 15},
		CerBeforeRoc: Duration{time.Second * 45},
		CerAfterRoc:  Duration{time.Second * 10},
	}
	flag.Var(&d.Rocon, "rocon-time", "ROCON execution time")
	flag.Var(&d.Rocoff, "rocoff-time", "ROCOFF execution time")
	flag.Var(&d.Ceron, "ceron-time", "CERON execution time (not used to scheduled CER)")
	flag.Var(&d.Ceroff, "ceroff-time", "CEROFF execution time (not used to scheduled CER)")
	flag.Var(&d.Wait, "rocon-wait", "wait time before starting ROCON")
	flag.Var(&d.Cer, "cer-time", "schedule CER TIME before entering eclipse")
	flag.Var(&d.Intersect, "cer-crossing", "intersection time to enable CER")
	flag.Var(&d.CerBefore, "cer-before", "schedule CERON before SAA")
	flag.Var(&d.CerAfter, "cer-after", "schedule CEROFF after SAA")
	flag.Var(&d.CerBeforeRoc, "cer-before-roc", "schedule CERON before ROC when conflict")
	flag.Var(&d.CerAfterRoc, "cer-after-roc", "schedule CEROFF after ROC when conflict")
	flag.Var(&d.AZM, "azm", "default AZM duration")
	flag.Var(&d.Margin, "roc-margin", "ROC margin")
	flag.Var(&d.Saa, "saa", "default SAA duration")
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
	ignore := flag.Bool("ignore", false, "ignore hazarduous blocks")
	elist := flag.Bool("list", false, "schedule list")
	plist := flag.Bool("list-periods", false, "periods list")
	ingest := flag.Bool("ingest", false, "")
	version := flag.Bool("version", false, "print version and exists")
	flag.Parse()

	if *version {
		fmt.Fprintf(os.Stderr, "%s-%s (%s)\n", Program, Version, BuildTime)
		return
	}

	b, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil && *baseTime != "" {
		Exit(badUsage("base-time format invalid"))
	}
	if b.IsZero() {
		b = DefaultBaseTime
	}
	var s *Schedule
	if *config {
		s, err = loadFromConfig(flag.Arg(0), &d, &fs, *ingest)
	} else {
		switch flag.NArg() {
		default:
			s, err = Open(flag.Arg(0), *resolution)
			if err == nil {
				fs.Path = flag.Arg(0)
			}
		case 0:
			s, err = OpenReader(os.Stdin, *resolution)
		}
	}
	if err != nil {
		Exit(err)
	}
	if *plist && !*ingest {
		ListPeriods(s, b)
		return
	}
	if *elist && !*ingest {
		if err := ListEntries(s, b, d, fs, *ignore); err != nil {
			Exit(err)
		}
		return
	}
	if err := fs.Can(); err != nil {
		Exit(err)
	}
	dumpSettings(d, fs)

	var w io.Writer
	digest := md5.New()
	switch f, err := os.Create(fs.Alliop); {
	case err == nil:
		w = io.MultiWriter(f, digest)
		defer f.Close()
	case err != nil && fs.Alliop == "":
		fs.Alliop = "alliop"
		w = io.MultiWriter(digest, os.Stdout)
	default:
		Exit(checkError(err, nil))
	}
	var es []*Entry
	if files := flag.Args(); *ingest {
		if len(files) <= 1 {
			Exit(badUsage("no files to ingest"))
		}
		if *config {
			files = files[1:]
		}
		es, err = ingestFiles(files, b)
	} else {
		es, err = s.Filter(b).Schedule(d, fs.CanROC(), fs.CanCER())
	}
	if err != nil {
		Exit(checkError(err, nil))
	}
	if len(es) == 0 {
		return
	}
	first, last := es[0], es[len(es)-1]
	log.Printf("first command (%s) at %s (%d)", first.Label, first.When.Format(timeFormat), SOY(first.When))
	log.Printf("last command (%s) at %s (%d)", last.Label, last.When.Format(timeFormat), SOY(last.When))
	b = es[0].When.Add(-Five)
	writePreamble(w, b)
	if err := writeMetadata(w, fs); err != nil {
		Exit(err)
	}
	ms, err := writeSchedule(w, es, b, fs)
	if err != nil {
		Exit(err)
	}
	for n, c := range ms {
		log.Printf("%s scheduled: %d", n, c)
	}
	_, tr := TimeROC(es, d)
	log.Printf("MXGS-ROC total time: %s", tr)
	_, tc := TimeCER(es, d)
	log.Printf("MMIA-CER total time: %s", tc)
	log.Printf("md5 %s: %x", fs.Alliop, digest.Sum(nil))

	if err := writeList(fs.Instrlist, fs.CanROC() && tr > 0, fs.CanCER() && tc > 0); err != nil {
		Exit(err)
	}
}

func loadFromConfig(file string, d *delta, fs *fileset, ingest bool) (*Schedule, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, checkError(err, nil)
	}
	defer r.Close()

	c := struct {
		Resolution Duration `toml:"resolution"`
		Trajectory string   `toml:"path"`

		Alliop  string `toml:"alliop"`
		Instr   string `toml:"instrlist"`
		Comment bool   `toml:"keep-comment"`

		Delta    *delta   `toml:"delta"`
		Commands *fileset `toml:"commands"`
	}{
		Delta:    d,
		Commands: fs,
	}
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return nil, badUsage(fmt.Sprintf("invalid configuration file: %v", err))
	}
	fs.Path, fs.Alliop, fs.Instrlist, fs.Keep = c.Trajectory, c.Alliop, c.Instr, c.Comment
	if ingest {
		return nil, nil
	}
	if c.Trajectory != "" {
		return Open(c.Trajectory, c.Resolution.Duration)
	}
	return OpenReader(os.Stdin, c.Resolution.Duration)
}

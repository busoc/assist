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
	"strings"
	"time"

	"github.com/midbel/toml"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const (
	Version   = "1.0.3"
	BuildTime = "2019-01-18 11:30:00"
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
		os.Exit(2)
	}

	b, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil && *baseTime != "" {
		log.Fatalln(err)
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
		log.Fatalln(err)
	}
	if *plist && !*ingest {
		ListPeriods(s, b)
		return
	}
	if *elist && !*ingest {
		if err := ListEntries(s, b, d, fs, *ignore); err != nil {
			log.Fatalln(err)
		}
		return
	}
	if err := fs.Can(); err != nil {
		log.Fatalln(err)
	}
	log.Printf("%s-%s (build: %s)", Program, Version, BuildTime)
	log.Printf("settings: AZM duration: %s", d.AZM.Duration)
	log.Printf("settings: ROCON time: %s", d.Rocon.Duration)
	log.Printf("settings: ROCOFF time: %s", d.Rocoff.Duration)
	log.Printf("settings: CER time: %s", d.Cer.Duration)
	log.Printf("settings: CERON time: %s", d.Ceron.Duration)
	log.Printf("settings: CEROFF time: %s", d.Ceroff.Duration)
	log.Printf("settings: CER crossing duration: %s", d.Intersect.Duration)

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
		log.Fatalln(err)
	}
	switch f, err := os.Create(fs.Instrlist); {
	case err == nil:
		digest := md5.New()
		w := io.MultiWriter(f, digest)
		defer f.Close()

		if fs.CanROC() {
			fmt.Fprintln(w, InstrMXGS)
		}
		if fs.CanCER() {
			fmt.Fprintln(w, InstrMMIA)
		}
		log.Printf("md5 %s: %x", fs.Instrlist, digest.Sum(nil))
	case err != nil && fs.Instrlist == "":
	default:
		log.Fatalln(err)
	}
	var es []*Entry
	if *ingest {
		fs := flag.Args()
		if len(fs) <= 1 {
			log.Fatalln("no files to ingest")
		}
		if *config {
			fs = fs[1:]
		}
		es, err = ingestFiles(fs, b)
	} else {
		es, err = s.Filter(b).Schedule(d, fs.CanROC(), fs.CanCER())
	}
	if err != nil {
		log.Fatalln(err)
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
		log.Fatalln(err)
	}
	ms, err := writeSchedule(w, es, b, fs)
	if err != nil {
		log.Fatalln(err)
	}
	for n, c := range ms {
		log.Printf("%s scheduled: %d", n, c)
	}
	log.Printf("md5 %s: %x", fs.Alliop, digest.Sum(nil))
}

func loadFromConfig(file string, d *delta, fs *fileset, ingest bool) (*Schedule, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
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
		return nil, err
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

func writeSchedule(w io.Writer, es []*Entry, when time.Time, fs fileset) (map[string]int, error) {
	cid := 1
	var err error

	ms := make(map[string]int)
	for _, e := range es {
		if e.When.Before(when) {
			continue
		}
		delta := e.When.Sub(when)
		switch e.Label {
		case ROCON:
			if !fs.CanROC() {
				return nil, missingFile("ROC")
			}
			cid, delta, err = prepareCommand(w, fs.Rocon, cid, e.When, delta, fs.Keep)
			ms[e.Label]++
		case ROCOFF:
			if !fs.CanROC() {
				return nil, missingFile("ROC")
			}
			cid, delta, err = prepareCommand(w, fs.Rocoff, cid, e.When, delta, fs.Keep)
			ms[e.Label]++
		case CERON:
			if !fs.CanCER() {
				return nil, missingFile("CER")
			}
			ms[e.Label]++
			cid, delta, err = prepareCommand(w, fs.Ceron, cid, e.When, delta, fs.Keep)
		case CEROFF:
			if !fs.CanCER() {
				return nil, missingFile("CER")
			}
			ms[e.Label]++
			cid, delta, err = prepareCommand(w, fs.Ceroff, cid, e.When, delta, fs.Keep)
		}
		if err != nil {
			return nil, err
		}
	}
	return ms, nil
}

func writePreamble(w io.Writer, when time.Time) {
	year := when.AddDate(0, 0, -when.YearDay()+1).Truncate(Day).Add(Leap)
	stamp := when.Add(Leap)

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

func writeMetadata(w io.Writer, fs fileset) error {
	for _, f := range []string{fs.Path, fs.Rocon, fs.Rocoff, fs.Ceron, fs.Ceroff} {
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
		log.Printf("%s: md5 = %x, lastmod: %s, size: %d bytes", f, digest.Sum(nil), mod, s.Size())
		fmt.Fprintf(w, "# %s: md5 = %x, lastmod: %s, size : %d bytes", f, digest.Sum(nil), mod, s.Size())
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

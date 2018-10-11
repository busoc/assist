package main

import (
	"flag"
	"fmt"
	"log"
	"os"
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

var (
	ExecutionTime   time.Time
	DefaultBaseTime time.Time
)

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
	flag.DurationVar(&d.Rocon, "delta-rocon", 50*time.Second, "delta ROC margin time (10s)")
	flag.DurationVar(&d.Rocoff, "delta-rocoff", 80*time.Second, "delta ROC margin time (80s)")
	flag.DurationVar(&d.Cer, "delta-cer", DefaultDeltaTime, "delta CER margin time (30s)")
	flag.DurationVar(&d.Intersect, "i", DefaultIntersectTime, "intersection time (2m)")
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
	for i, e := range s.Entries {
		log.Printf("%3d | %7s | %s", i+1, e.Label, e.When.Truncate(time.Second).Format(timeFormat))
	}
}

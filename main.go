package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000000"

const (
	Version   = "2.0.0"
	BuildTime = "2021-01-05 08:30:00"
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
	ast := Default()
	if err := ast.LoadAndFilter(flag.Arg(0), base); err != nil {
		Exit(checkError(err, nil))
	}
	if *plist {
		ast.PrintPeriods()
		return
	}
	if *elist {
		ast.PrintEntries()
		return
	}
	err = ast.Create()
	Exit(checkError(err, nil))
}

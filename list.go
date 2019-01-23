package main

import (
	"fmt"
	"time"
)

func ListPeriods(s *Schedule, b time.Time) error {
	s = s.Filter(b)
	for i, p := range s.Periods() {
		fmt.Printf("%3d | %-8s | %s | %s | %s", i, p.Label, p.Starts.Format("2006-01-02T15:04:05"), p.Ends.Format("2006-01-02T15:04:05"), p.Duration())
		fmt.Println()
	}
	return nil
}

func ListEntries(s *Schedule, b time.Time, d delta, fs fileset, ignore bool) error {
	s.Ignore = ignore
	canROC, canCER := fs.CanROC(), fs.CanCER()
	if !canROC && !canCER {
		canROC, canCER = true, true
	}
	es, err := s.Filter(b).Schedule(d, canROC, canCER)
	if err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	first, last := es[0], es[len(es)-1]
	fmt.Printf("%3s | %s | %-9s | %-9s | %-20s | %-20s", "#", "?", "TYPE", "SOY (GPS)", "START (GMT)", "END (GMT)")
	fmt.Println()
	fmt.Printf("%3d | %s | %-9s | %-9d | %-20s | %-20s", 0, " ", "SCHEDULE", SOY(first.When.Add(-Five)), first.When.Add(-Five).Format("2006-01-02T15:04:05"), last.When.Format("2006-01-02T15:04:05"))
	fmt.Println()

	for i, e := range es {
		var to time.Time
		switch e.Label {
		case ROCON:
			to = e.When.Add(d.Rocon.Duration)
		case ROCOFF:
			to = e.When.Add(d.Rocoff.Duration)
		case CERON:
			to = e.When.Add(d.Ceron.Duration)
		case CEROFF:
			to = e.When.Add(d.Ceroff.Duration)
		}
		conflict := "-"
		if e.Warning {
			conflict = "!"
		}

		fmt.Printf("%3d | %s | %-9s | %-9d | %-20s | %-20s", i+1, conflict, e.Label, e.SOY(), e.When.Format("2006-01-02T15:04:05"), to.Format("2006-01-02T15:04:05"))
		fmt.Println()
	}
	return nil
}

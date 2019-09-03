package main

import (
	"fmt"
	"time"
)

func ListPeriods(s *Schedule, b time.Time) error {
	s = s.Filter(b)

	var (
		ed, ad time.Duration
		ec, ac int
	)
	for i, p := range s.Periods() {
		d := p.Duration()
		fmt.Printf("%3d | %-8s | %s | %s | %s", i, p.Label, p.Starts.Format("2006-01-02T15:04:05"), p.Ends.Format("2006-01-02T15:04:05"), d)
		fmt.Println()
		switch p.Label {
		case "saa":
			ad += d
			ac++
		case "eclipse":
			ed += d
			ec++
		}
	}
	fmt.Println()
	fmt.Printf("eclipse total time: %s (%d)", ed, ec)
	fmt.Println()
	fmt.Printf("saa total time: %s (%d)", ad, ac)
	fmt.Println()
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
	fmt.Println()
	var (
		t time.Duration
		p int
	)

	p, t = TimeROC(es, d)
	fmt.Printf("MXGS-ROC total time: %s (%d)", t, p)
	fmt.Println()

	p, t = TimeCER(es, d)
	fmt.Printf("MMIA-CER total time: %s (%d)", t, p)
	fmt.Println()

	return nil
}

func TimeROC(es []*Entry, d delta) (int, time.Duration) {
	var (
		i, p int
		t    time.Duration
	)
	for i < len(es) {
		if es[i].Label != ROCON {
			i++
			continue
		}
		j := i + 1
		for j < len(es) {
			if es[j].Label != ROCOFF {
				j++
				continue
			}
			t += es[j].When.Sub(es[i].When.Add(d.Rocon.Duration))
			p++
			i = j + 1
			break
		}
		if j >= len(es) {
			break
		}
	}
	return p, t
}

func TimeCER(es []*Entry, d delta) (int, time.Duration) {
	var (
		i, p int
		t    time.Duration
	)
	for i < len(es) {
		if es[i].Label != CEROFF {
			i++
			continue
		}
		j := i + 1
		for j < len(es) {
			if es[j].Label != CERON {
				j++
				continue
			}
			t += es[j].When.Sub(es[i].When.Add(d.Ceroff.Duration))
			p++
			i = j + 1
			break
		}
		if j >= len(es) {
			break
		}
	}
	return p, t
}

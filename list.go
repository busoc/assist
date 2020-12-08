package main

import (
	"fmt"
	"time"
)

func ListPeriods(s *Schedule) error {
	var (
		ed, ad, xd time.Duration
		ec, ac, xc int
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
		case "aurora":
			xd += d
			xc++
		}
	}
	fmt.Println()
	fmt.Printf("eclipse total time: %s (%d)", ed, ec)
	fmt.Println()
	fmt.Printf("saa total time: %s (%d)", ad, ac)
	fmt.Println()
	fmt.Printf("aurora total time: %s (%d)", xd, xc)
	fmt.Println()
	return nil
}

func ListEntries(s *Schedule, d delta, fs fileset, ignore bool) error {
	s.Ignore = ignore
	canROC, canCER, canACS := fs.CanROC(), fs.CanCER(), fs.CanACS()
	if !canROC && !canCER && !canACS {
		canROC, canCER, canACS = true, true, true
	}
	es, err := s.Schedule(d, canROC, canCER, canACS)
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
		case ACSON, ACSOFF:
			to = e.When.Add(d.AcsTime.Duration)
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

	p, t = TimeACS(es, d)
	fmt.Printf("MXGS-ACS total time: %s (%d)", t, p)
	fmt.Println()

	p, t = TimeCER(es, d)
	fmt.Printf("MMIA-CER total time: %s (%d)", t, p)
	fmt.Println()

	return nil
}

func TimeACS(es []*Entry, d delta) (int, time.Duration) {
	var (
		i, p int
		t    time.Duration
	)
	for i < len(es) {
		i++
		if es[i-1].Label != ACSON && es[i-1].Label != ACSOFF {
			continue
		}
		p++
		t += d.AcsTime.Duration
	}
	return p, t
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

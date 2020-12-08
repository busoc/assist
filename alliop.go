package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"
)

const (
	InstrMMIA = "MMIA 129"
	InstrMXGS = "MXGS 128"
)

func writeList(file string, roc, cer bool) error {
	switch f, err := os.Create(file); {
	case err == nil:
		defer f.Close()

		digest := md5.New()
		w := io.MultiWriter(f, digest)

		if roc {
			fmt.Fprintln(w, InstrMXGS)
		}
		if cer {
			fmt.Fprintln(w, InstrMMIA)
		}
		log.Printf("md5 %s: %x", file, digest.Sum(nil))
	case err != nil && file == "":
	default:
		return checkError(err, nil)
	}
	return nil
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
		case ACSON:
			if !fs.CanACS() {
				return nil, missingFile("ACS")
			}
			ms[e.Label]++
			cid, delta, err = prepareCommand(w, fs.Acson, cid, e.When, delta, fs.Keep)
		case ACSOFF:
			if !fs.CanACS() {
				return nil, missingFile("ACS")
			}
			ms[e.Label]++
			cid, delta, err = prepareCommand(w, fs.Acsoff, cid, e.When, delta, fs.Keep)
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
	for _, f := range []string{fs.Path, fs.Rocon, fs.Rocoff, fs.Ceron, fs.Ceroff, fs.Acson, fs.Acsoff} {
		if f == "" {
			continue
		}
		r, err := os.Open(f)
		if err != nil {
			return checkError(err, nil)
		}
		defer r.Close()

		digest := md5.New()
		if _, err := io.Copy(digest, r); err != nil {
			return checkError(err, nil)
		}
		s, err := r.Stat()
		if err != nil {
			return checkError(err, nil)
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
		return cid, 0, checkError(err, nil)
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
	switch e := s.Err(); e {
	case bufio.ErrTooLong, bufio.ErrNegativeAdvance, bufio.ErrAdvanceTooFar:
		err = badUsage(fmt.Sprintf("%s: processing failed (%v)", file, e))
	default:
		if e != nil {
			err = badUsage(err.Error())
		}
	}
	fmt.Fprintln(w)
	return cid, elapsed, err
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

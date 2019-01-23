package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func ingestFiles(files []string, b time.Time) ([]*Entry, error) {
	ingest := func(file string) ([]*Entry, error) {
		r, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		rs := csv.NewReader(r)
		rs.Comment = PredictComment
		rs.Comma = '|'
		rs.FieldsPerRecord = 6
		rs.TrimLeadingSpace = true

		var es []*Entry
		for {
			rs, err := rs.Read()
			if rs == nil && err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			switch n := strings.TrimSpace(rs[2]); n {
			case ROCON, ROCOFF, CERON, CEROFF:
				e := Entry{Label: n}
				if e.When, err = time.Parse("2006-01-02T15:04:05", strings.TrimSpace(rs[4])); err != nil {
					return nil, err
				}
				if !b.IsZero() && e.When.Before(b) {
					continue
				}
				es = append(es, &e)
			case "SCHEDULE":
			default:
				return nil, fmt.Errorf("invalid command name: %s", n)
			}
		}
		return es, nil
	}
	var es []*Entry
	for _, f := range files {
		vs, err := ingest(f)
		if err != nil {
			return nil, err
		}
		es = append(es, vs...)
	}
	return es, nil
}

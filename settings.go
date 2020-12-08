package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	ROCON  = "ROCON"
	ROCOFF = "ROCOFF"
	CERON  = "CERON"
	CEROFF = "CEROFF"
	ACSON  = "ACSON"
	ACSOFF = "ACSOFF"
)

const (
	ALLIOP = "alliop.txt"
	INSTR  = "instrlist.txt"
)

var (
	ExecutionTime   time.Time
	DefaultBaseTime time.Time
)

type Shape interface {
	IsZero() bool
	Contains(float64, float64) bool
	fmt.Stringer
}

type Rect struct {
	North float64 `toml:"north"`
	South float64 `toml:"south"`
	West  float64 `toml:"west"`
	East  float64 `toml:"east"`
}

func (r Rect) String() string {
	return fmt.Sprintf("%.0fN %.0fS %.0fW %.0fE", r.North, r.South, r.East, r.West)
}

func (r Rect) IsZero() bool {
	return r.North == r.South || r.West == r.East
}

func (r Rect) Contains(lat, lng float64) bool {
	if r.IsZero() || !r.isValid() {
		return false
	}
	return lat <= r.North && lat >= r.South && lng <= r.East && lng >= r.West
}

func (r Rect) isValid() bool {
	return r.South < r.North && r.West < r.East
}

type Area struct {
	shapes []Shape
}

func NewArea(as ...Shape) Shape {
	return Area{
		shapes: append([]Shape{}, as...),
	}
}

func (a Area) String() string {
	var b strings.Builder
	for i, s := range a.shapes {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("(")
		b.WriteString(s.String())
		b.WriteString(")")
	}
	return b.String()
}

func (a Area) IsZero() bool {
	for _, s := range a.shapes {
		if !s.IsZero() {
			return false
		}
	}
	return true
}

func (a Area) Contains(lat, lng float64) bool {
	for _, s := range a.shapes {
		if s.Contains(lat, lng) {
			return true
		}
	}
	return false
}

func dumpSettings(d delta, fs fileset) {
	log.Printf("%s-%s (build: %s)", Program, Version, BuildTime)
	log.Printf("settings: AZM duration: %s", d.AZM.Duration)
	log.Printf("settings: ROCON time: %s", d.Rocon.Duration)
	log.Printf("settings: ROCOFF time: %s", d.Rocoff.Duration)
	log.Printf("settings: CER time: %s", d.Cer.Duration)
	log.Printf("settings: CERON time: %s", d.Ceron.Duration)
	log.Printf("settings: CEROFF time: %s", d.Ceroff.Duration)
	log.Printf("settings: CER crossing duration: %s", d.Intersect.Duration)
	log.Printf("settings: ACS min night duration: %s", d.AcsNight.Duration)
	log.Printf("settings: ACS duration: %s", d.AcsTime.Duration)
}

type Duration struct {
	time.Duration
}

func (d *Duration) String() string {
	return d.Duration.String()
}

func (d *Duration) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err == nil {
		d.Duration = v
	}
	return err
}

type delta struct {
	Rocon     Duration `toml:"rocon"`
	Rocoff    Duration `toml:"rocoff"`
	Ceron     Duration `toml:"ceron"`
	Ceroff    Duration `toml:"ceron"`
	Margin    Duration `toml:"margin"`
	Cer       Duration `toml:"cer"`
	Wait      Duration `toml:"wait"`
	Intersect Duration `toml:"crossing"`
	AZM       Duration `toml:"azm"`
	Saa       Duration `toml:"saa"`

	CerBefore    Duration `toml:"cer-before"`
	CerAfter     Duration `toml:"cer-after"`
	CerBeforeRoc Duration `toml:"cer-before-roc"`
	CerAfterRoc  Duration `toml:"cer-after-roc"`

	AcsNight Duration `toml:"acs-night"`
	AcsTime  Duration `toml:"acs-duration"`
}

type fileset struct {
	Path string `toml:"-"`

	Rocon  string `toml:"rocon"`
	Rocoff string `toml:"rocoff"`
	Ceron  string `toml:"ceron"`
	Ceroff string `toml:"ceroff"`
	Acson  string `toml:"acson"`
	Acsoff string `toml:"acsoff"`
	Keep   bool   `toml:"-"`

	Alliop    string `toml:"-"`
	Instrlist string `toml:"-"`
}

func (f fileset) CanROC() bool {
	return f.Rocon != "" && f.Rocoff != ""
}

func (f fileset) CanCER() bool {
	return f.Ceron != "" && f.Ceroff != ""
}

func (f fileset) CanACS() bool {
	return f.Acson != "" && f.Acsoff != ""
}

func (f fileset) Empty() bool {
	return f.Rocon == "" && f.Rocoff == "" && f.Ceron == "" && f.Ceroff == "" && f.Acson == "" && f.Acsoff == ""
}

func (f fileset) Can() error {
	if (f.Rocon == "" && f.Rocoff != "") || (f.Rocon != "" && f.Rocoff == "") {
		return missingFile("ROC")
	}
	if f.Rocon != "" && f.Rocoff != "" && f.Rocon == f.Rocoff {
		return sameFile("ROC")
	}
	if (f.Ceron == "" && f.Ceroff != "") || (f.Ceron != "" && f.Ceroff == "") {
		return missingFile("CER")
	}
	if f.Ceron != "" && f.Ceroff != "" && f.Ceron == f.Ceroff {
		return sameFile("CER")
	}
	if f.Empty() {
		return genericErr("no command files given")
	}
	return nil
}

package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	rocDefault = RocOption{
		TimeSAA:      NewDuration(10),
		TimeAZM:      NewDuration(40),
		TimeOn:       NewDuration(50),
		TimeOff:      NewDuration(80),
		TimeBetween:  NewDuration(120),
		WaitBeforeOn: NewDuration(100),
	}
	cerDefault = CerOption{
		SwitchTime:      NewDuration(0),
		SaaCrossingTime: NewDuration(120),
		BeforeSaa:       NewDuration(50),
		AfterSaa:        NewDuration(15),
		BeforeRoc:       NewDuration(45),
		AfterRoc:        NewDuration(10),
		TimeOn:          NewDuration(40),
		TimeOff:         NewDuration(40),
	}
	aurDefault = AuroraOption{
		Night: NewDuration(180),
		Time:  NewDuration(5),
	}
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

type Duration struct {
	time.Duration
}

func NewDuration(sec int) Duration {
	d := time.Second * time.Duration(sec)
	return Duration{d}
}

func (d *Duration) IsZero() bool {
	return d.Duration == 0
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

type Fileset struct {
	On  string `toml:"on-cmd-file"`
	Off string `toml:"off-cmd-file"`
}

func (f Fileset) Check() error {
	if f.On == f.Off {
		return sameFile("cmd-file")
	}
	if i, err := os.Stat(f.On); err != nil || !i.Mode().IsRegular() {
		return missingFile(f.On)
	}
	if i, err := os.Stat(f.Off); err != nil || !i.Mode().IsRegular() {
		return missingFile(f.Off)
	}
	return nil
}

func (f Fileset) Can() bool {
	return f.Check() == nil
}

type RocOption struct {
	Fileset `toml:"commands"`

	TimeSAA      Duration `toml:"saa-duration"`
	TimeAZM      Duration `toml:"azm-duration"`
	TimeOn       Duration `toml:"on-duration"`
	TimeOff      Duration `toml:"off-duration"`
	TimeBetween  Duration `toml:"time-between-onoff"`
	WaitBeforeOn Duration `toml:"wait-before-on"`
}

func (r RocOption) Can() bool {
	return r.Fileset.Can() && !r.TimeOn.IsZero() && !r.TimeOff.IsZero()
}

type CerOption struct {
	Fileset `toml:"commands"`

	TimeOn  Duration `toml:"on-duration"`
	TimeOff Duration `toml:"off-duration"`

	BeforeSaa Duration `toml:"time-before-saa"`
	AfterSaa  Duration `toml:"time-after-saa"`
	BeforeRoc Duration `toml:"time-before-roc"`
	AfterRoc  Duration `toml:"time-after-roc"`

	SaaCrossingTime Duration `toml:"saa-crossing-time"`
	SwitchTime      Duration `toml:"switch-onoff-time"`
}

func (c CerOption) Can() bool {
	return c.Fileset.Can()
}

type AuroraOption struct {
	Fileset `toml:"commands"`

	Night Duration `toml:"min-night-duration"`
	Time  Duration `toml:"duration"`
	Areas []Rect   `toml:"areas"`
}

func (a AuroraOption) Can() bool {
	return a.Fileset.Can() && !a.Night.IsZero() && len(a.Areas) > 0
}

func (a AuroraOption) Accept(p *Period) bool {
	return p.Duration() >= (a.Night.Duration + 2*a.Time.Duration)
}

func (a AuroraOption) Area() Shape {
	rs := make([]Shape, len(a.Areas))
	for i := range a.Areas {
		rs[i] = a.Areas[i]
	}
	return NewArea(rs...)
}

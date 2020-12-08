package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"syscall"
)

const (
	EIO    = 5
	EINVAL = 22
)

const (
	GenericErrCode = 5000 + iota
	MissingFileErrCode
	SameFileErrCode
)

type Error struct {
	Cause error
	Code  int
}

func (e *Error) Error() string {
	return e.Cause.Error()
}

func Exit(e error) {
	if e == nil {
		return
	}
	fmt.Println(e)
	if e, ok := e.(*Error); ok {
		os.Exit(e.Code)
	} else {
		os.Exit(GenericErrCode)
	}
}

func checkError(err, parent error) error {
	if err == nil {
		return nil
	}
	switch e := err.(type) {
	case *csv.ParseError:
		return badUsage(e.Error())
	case *os.PathError:
		return checkError(e.Err, err)
	case syscall.Errno:
		if parent != nil {
			err = parent
		}
		return &Error{Cause: err, Code: int(e)}
	default:
		return err
	}
}

func badUsage(n string) error {
	e := Error{
		Cause: fmt.Errorf(n),
		Code:  EINVAL,
	}
	return &e
}

func floatBadSyntax(i int, v string) error {
	e := Error{
		Cause: fmt.Errorf("number badly formatted at row %d (%s)", i+1, v),
		Code:  EINVAL,
	}
	return &e
}

func timeBadSyntax(i int, v string) error {
	e := Error{
		Cause: fmt.Errorf("time badly formatted at row %d (%s)", i+1, v),
		Code:  EINVAL,
	}
	return &e
}

func genericErr(n string) error {
	e := Error{
		Cause: fmt.Errorf(n),
		Code:  GenericErrCode,
	}
	return &e
}

func sameFile(n string) error {
	e := Error{
		Cause: fmt.Errorf("%s: same file for on/off", strings.ToUpper(n)),
		Code:  SameFileErrCode,
	}
	return &e
}

func missingFile(n string) error {
	e := Error{
		Cause: fmt.Errorf("%s: files should be provided by pair (on/off)", strings.ToUpper(n)),
		Code:  MissingFileErrCode,
	}
	return &e
}

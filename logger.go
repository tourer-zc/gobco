package main

import (
	"fmt"
	"io"
)

// logger provides basic logging and error checking.
type logger struct {
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
}

func (l *logger) init(stdout io.Writer, stderr io.Writer) {
	l.stdout = stdout
	l.stderr = stderr
}

func (l *logger) check(err error) {
	if err != nil {
		l.errf("%s", err)
		exit(1)
	}
}

func (l *logger) outf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(l.stdout, format+"\n", args...)
}

func (l *logger) errf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(l.stderr, format, args...)
}

func (l *logger) verbosef(format string, args ...interface{}) {
	if l.verbose {
		l.errf(format, args...)
	}
}

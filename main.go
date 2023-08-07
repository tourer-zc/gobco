package main

import (
	"io"
	"os"
)

const version = "1.0.2-snapshot"

var exit = os.Exit

func main() {
	exit(gobcoMain(os.Stdout, os.Stderr, os.Args...))
}

func gobcoMain(stdout, stderr io.Writer, args ...string) int {
	g := newGobco(stdout, stderr)
	g.parseCommandLine(args)
	g.prepareTmp()
	g.instrument()
	g.runGoTest()
	g.printOutput()
	//g.cleanUp()
	return g.exitCode
}

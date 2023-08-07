package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goTest groups the functions that run 'go test' with the proper arguments.
type goTest struct{}

// argInfo describes the properties of an item that will be instrumented.
//
// If it is inside GOPATH, it or its containing directory is copied, otherwise
// the whole Go module will be copied.
//
// If it is a file, only that file is instrumented, otherwise the whole package
// is instrumented. Even in case of a single file, the whole directory is
// copied though.
type argInfo struct {
	// From the command line, using either '/' or '\\' as separator.
	arg string

	// Either arg if it is a directory, or its containing directory.
	// Either absolute, or relative to the current working directory.
	//
	// This is the directory from which the code is instrumented. The paths
	// to the files in this directory will end up in the coverage output.
	argDir string

	// Whether arg is a module (true) or a traditional package (false).
	module bool

	// The directory that will be copied to the build environment.
	// Either absolute, or relative to the current working directory.
	// For modules, it is the module root, so that go.mod is copied as well.
	// For other packages it is the package directory itself.
	copySrc string

	// The copy destination, relative to tmpdir.
	// For modules, it is some directory outside 'gopath/src',
	// traditional packages are copied to 'gopath/src/$pkgname'.
	copyDst string

	// The single file in which to instrument the code, relative to instrDir,
	// or "" to instrument the whole package.
	instrFile string

	// The directory where the instrumented code is saved, relative to tmpdir.
	// The directory in which to run 'go test', relative to tmpdir.
	instrDir string
}

type condition struct {
	Start      string
	Code       string
	TrueCount  int
	FalseCount int
}

func (t goTest) run(
	arg argInfo,
	extraArgs []string,
	verbose bool,
	gopaths string,
	statsFilename string,
	e *buildEnv,
) int {
	args := t.args(verbose, extraArgs)
	goTest := exec.Command("go", args[1:]...)
	goTest.Stdout = os.Stdout
	goTest.Stderr = e.stderr
	goTest.Dir = e.file(arg.instrDir)
	goTest.Env = t.env(e.tmpdir, gopaths, statsFilename)

	cmdline := strings.Join(args, " ")
	e.verbosef("Running %q in %q", cmdline, goTest.Dir)

	err := goTest.Run()
	if err != nil {
		e.errf("%s", err)
		return 1
	} else {
		e.verbosef("Finished %s", cmdline)
		return 0
	}
}

func (goTest) args(verbose bool, extraArgs []string) []string {
	args := []string{"go", "test"}

	if verbose {
		// The -v is necessary to produce any output at all.
		// Without it, most of the log output is suppressed.
		args = append(args, "-v")
	}

	// Work around test result caching which does not apply anyway,
	// since the instrumented files are written to a new directory
	// each time.
	//
	// Without this option, "go test" sometimes needs twice the time.
	args = append(args, "-test.count", "1")

	args = append(args, ".")

	// 'go test' allows flags even after packages.
	args = append(args, extraArgs...)

	return args
}

func (goTest) env(tmpdir, gopaths, statsFilename string) []string {

	var env []string

	for _, envVar := range os.Environ() {
		if gopaths == "" && strings.HasPrefix(envVar, "GOPATH=") {
			continue
		}
		env = append(env, envVar)
	}

	if gopaths != "" {
		gopathDir := filepath.Join(tmpdir, "gopath")
		gopath := gopathDir + string(filepath.ListSeparator) + gopaths
		env = append(env, "GOPATH="+gopath)
		env = append(env, "GO111MODULE=off")
	}

	env = append(env, "GOBCO_STATS="+statsFilename)

	return env
}

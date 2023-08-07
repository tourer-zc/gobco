package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type gobco struct {
	listAll     bool
	immediately bool
	keep        bool
	coverTest   bool

	goTestArgs []string
	args       []argInfo

	statsFilename string

	exitCode int

	logger
	buildEnv
}

func newGobco(stdout io.Writer, stderr io.Writer) *gobco {
	var g gobco
	g.logger.init(stdout, stderr)
	g.buildEnv.init(&g.logger)
	return &g
}

func (g *gobco) parseCommandLine(argv []string) {
	args := g.parseOptions(argv)
	g.parseArgs(args)
}

func (g *gobco) parseOptions(argv []string) []string {
	var help, ver bool

	flags := flag.NewFlagSet(filepath.Base(argv[0]), flag.ContinueOnError)
	flags.BoolVar(&help, "help", false,
		"print the available command line options")
	flags.BoolVar(&g.immediately, "immediately", false,
		"persist the coverage immediately at each check point")
	flags.BoolVar(&g.keep, "keep", false,
		"don't remove the temporary working directory")
	flags.BoolVar(&g.listAll, "list-all", false,
		"at finish, print also those conditions that are fully covered")
	flags.StringVar(&g.statsFilename, "stats", "",
		"load and persist the JSON coverage data to this `file`")
	flags.Var(newSliceFlag(&g.goTestArgs), "test",
		"pass the `option` to \"go test\", such as -vet=off")
	flags.BoolVar(&g.verbose, "verbose", false,
		"show progress messages")
	flags.BoolVar(&g.coverTest, "cover-test", false,
		"cover the test code as well")
	flags.BoolVar(&ver, "version", false,
		"print the gobco version")

	flags.SetOutput(g.stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(flags.Output(),
			"usage: %s [options] package...\n", flags.Name())
		flags.PrintDefaults()
		g.exitCode = 2
	}

	err := flags.Parse(argv[1:])
	if g.exitCode != 0 {
		exit(g.exitCode)
	}
	g.check(err)

	if help {
		flags.SetOutput(g.stdout)
		flags.Usage()
		exit(0)
	}

	if ver {
		g.outf("%s", version)
		exit(0)
	}

	return flags.Args()
}

func (g *gobco) parseArgs(args []string) {
	if len(args) == 0 {
		args = []string{"."}
	}

	if len(args) > 1 {
		panic("gobco: checking multiple packages doesn't work yet")
	}

	for _, arg := range args {
		arg = filepath.FromSlash(arg)
		g.args = append(g.args, g.classify(arg))
	}
}

// classify determines how to handle the argument, depending on whether it is
// a single file or directory, and whether it is located in a Go module or not.
func (g *gobco) classify(arg string) argInfo {
	st, err := os.Stat(arg)
	isDir := err == nil && st.IsDir()

	dir := arg
	base := ""
	if !isDir {
		dir = filepath.Dir(dir)
		base = filepath.Base(arg)
	}

	if moduleRoot, moduleRel := g.findInModule(dir); moduleRoot != "" {
		copyDst := "module-" + randomHex(8) // Must be outside 'gopath/'.
		packageDir := filepath.Join(copyDst, moduleRel)
		fmt.Println("==============", moduleRoot, copyDst, packageDir)
		return argInfo{
			arg:       arg,
			argDir:    dir,
			module:    true,
			copySrc:   moduleRoot,
			copyDst:   copyDst,
			instrFile: base,
			instrDir:  packageDir,
		}
	}

	if relDir := g.findInGopath(dir); relDir != "" {
		relDir := filepath.Join("gopath", relDir)
		return argInfo{
			arg:       arg,
			argDir:    dir,
			module:    false,
			copySrc:   dir,
			copyDst:   relDir,
			instrFile: base,
			instrDir:  relDir,
		}
	}

	g.check(fmt.Errorf("error: argument %q must be inside GOPATH", arg))
	panic("unreachable")
}

// findInGopath returns the directory relative to the enclosing GOPATH, if any.
func (g *gobco) findInGopath(arg string) string {
	gopaths := g.gopaths()

	abs, err := filepath.Abs(arg)
	g.check(err)

	for _, gopath := range filepath.SplitList(gopaths) {

		rel, err := filepath.Rel(gopath, abs)
		g.check(err)

		if strings.HasPrefix(rel, "src") {
			return rel
		}
	}
	return ""
}

func (g *gobco) gopaths() string {
	gopaths := os.Getenv("GOPATH")
	if gopaths != "" {
		return gopaths
	}

	home, err := os.UserHomeDir()
	g.check(err)
	return filepath.Join(home, "go")
}

func (g *gobco) findInModule(dir string) (moduleRoot, moduleRel string) {
	absDir, err := filepath.Abs(dir)
	g.check(err)

	abs := absDir
	for {
		if _, err := os.Lstat(filepath.Join(abs, "go.mod")); err == nil {
			rel, err := filepath.Rel(abs, absDir)
			g.check(err)

			root := abs
			if rel == "." {
				root = dir
			}

			return root, rel
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return "", ""
		}
		abs = parent
	}
}

// prepareTmp copies the source files to the temporary directory.
//
// Some of these files will later be overwritten by gobco.instrumenter.
func (g *gobco) prepareTmp() {
	if g.statsFilename != "" {
		var err error
		g.statsFilename, err = filepath.Abs(g.statsFilename)
		g.check(err)
	} else {
		g.statsFilename = g.file("gobco-counts.json")
	}

	// TODO: Research how "package/..." is handled by other go commands.
	for _, arg := range g.args {
		dstDir := g.file(arg.copyDst)
		g.check(copyDir(arg.copySrc, dstDir))
	}
}

func (g *gobco) instrument() {
	var in instrumenter
	in.immediately = g.immediately
	in.listAll = g.listAll
	in.coverTest = g.coverTest

	for _, arg := range g.args {
		instrDst := g.file(arg.instrDir)
		in.instrument(arg.argDir, arg.instrFile, instrDst)
		g.verbosef("Instrumented %s to %s", arg.arg, instrDst)
	}
}

func (g *gobco) runGoTest() {
	for _, arg := range g.args {
		gopaths := ""
		if !arg.module {
			gopaths = g.gopaths()
		}
		g.exitCode = goTest{}.run(
			arg,
			g.goTestArgs,
			g.verbose,
			gopaths,
			g.statsFilename,
			&g.buildEnv,
		)
	}
}

func (g *gobco) printOutput() {
	conds := g.load(g.statsFilename)

	cnt := 0
	for _, c := range conds {
		if c.TrueCount > 0 {
			cnt++
		}
		if c.FalseCount > 0 {
			cnt++
		}
	}

	g.outf("")
	g.outf("Branch coverage: %d/%d", cnt, len(conds)*2)

	for _, cond := range conds {
		g.printCond(cond)
	}
}

func (g *gobco) cleanUp() {
	if g.keep {
		g.errf("")
		g.errf("the temporary files are in %s", g.tmpdir)
	} else {
		err := os.RemoveAll(g.tmpdir)
		if err != nil {
			g.verbosef("%s", err)
		}
	}
}

func (g *gobco) load(filename string) []condition {
	file, err := os.Open(filename)
	g.check(err)

	defer func() {
		closeErr := file.Close()
		g.check(closeErr)
	}()

	var data []condition
	decoder := json.NewDecoder(bufio.NewReader(file))
	decoder.DisallowUnknownFields()
	g.check(decoder.Decode(&data))

	return data
}

func (g *gobco) printCond(cond condition) {

	trueCount := cond.TrueCount
	falseCount := cond.FalseCount
	start := cond.Start
	code := cond.Code

	if !g.listAll && trueCount > 0 && falseCount > 0 {
		return
	}

	capped := func(count int) int {
		if count > 1 {
			return 2
		}
		if count == 1 {
			return 1
		}
		return 0
	}

	switch 3*capped(trueCount) + capped(falseCount) {
	case 0:
		g.outf("%s: condition %q was never evaluated",
			start, code)
	case 1:
		g.outf("%s: condition %q was once false but never true",
			start, code)
	case 2:
		g.outf("%s: condition %q was %d times false but never true",
			start, code, falseCount)
	case 3:
		g.outf("%s: condition %q was once true but never false",
			start, code)
	case 4:
		g.outf("%s: condition %q was once true and once false",
			start, code)
	case 5:
		g.outf("%s: condition %q was once true and %d times false",
			start, code, falseCount)
	case 6:
		g.outf("%s: condition %q was %d times true but never false",
			start, code, trueCount)
	case 7:
		g.outf("%s: condition %q was %d times true and once false",
			start, code, trueCount)
	default:
		g.outf("%s: condition %q was %d times true and %d times false",
			start, code, trueCount, falseCount)
	}
}

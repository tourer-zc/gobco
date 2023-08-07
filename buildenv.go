package main

import (
	"os"
	"path/filepath"
)

// buildEnv describes the environment in which all interesting pieces of code
// are collected and instrumented.
type buildEnv struct {
	tmpdir string
	*logger
}

func (e *buildEnv) init(r *logger) {

	tmpdir := filepath.Join(os.TempDir(), "gobco-"+randomHex(8))

	r.check(os.MkdirAll(tmpdir, 0777))

	r.verbosef("The temporary working directory is %s", tmpdir)

	*e = buildEnv{tmpdir, r}
}

// file returns the absolute path of the given path, which is interpreted
// relative to the temporary directory.
func (e *buildEnv) file(rel string) string {
	return filepath.Join(e.tmpdir, filepath.FromSlash(rel))
}

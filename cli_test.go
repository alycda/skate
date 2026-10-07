package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// The scripts under testdata drive the real command tree: testscript.RunMain
// re-executes this test binary with main() as the `skate` command, so each
// `exec skate ...` is a fresh process, like a user's shell would give.
func TestMain(m *testing.M) {
	// On Windows, exec looks in the current directory before PATH. `go build`
	// leaves skate.exe in the package directory, where it would shadow the
	// testscript command and fail with "cannot run executable found relative
	// to current directory". Setting this (to any value) turns that lookup off.
	os.Setenv("NoDefaultCurrentDirectoryInExePath", "1")
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"skate": func() int {
			main()
			return 0
		},
	}))
}

// loaderVars let a ditto build find libdittoffi when it is not installed in a
// system path. testscript environments are otherwise hermetic.
var loaderVars = []string{"LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "DYLD_FALLBACK_LIBRARY_PATH"}

// normalizeNewlines rewrites CRLF to LF in the extracted fixtures. They are
// compared byte for byte with skate's output, which uses LF on every platform,
// but a Windows checkout with core.autocrlf rewrites them to CRLF. The repo's
// .gitattributes asks for LF; this covers clones made before that applied.
func normalizeNewlines(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")); len(n) != len(b) {
			return os.WriteFile(path, n, 0o666)
		}
		return nil
	})
}

func scriptParams(dir string, env ...string) testscript.Params {
	return testscript.Params{
		Dir: dir,
		Setup: func(e *testscript.Env) error {
			if err := normalizeNewlines(e.WorkDir); err != nil {
				return err
			}
			e.Setenv("SKATE_STORE", filepath.Join(e.WorkDir, "store"))
			for _, k := range loaderVars {
				if v, ok := os.LookupEnv(k); ok {
					e.Setenv(k, v)
				}
			}
			for _, kv := range env {
				k, v, _ := strings.Cut(kv, "=")
				e.Setenv(k, v)
			}
			return nil
		},
	}
}

// TestScripts covers behavior shared by every build.
func TestScripts(t *testing.T) {
	testscript.Run(t, scriptParams("testdata/script"))
}

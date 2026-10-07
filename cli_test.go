package main

import (
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

func scriptParams(dir string, env ...string) testscript.Params {
	return testscript.Params{
		Dir: dir,
		Setup: func(e *testscript.Env) error {
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

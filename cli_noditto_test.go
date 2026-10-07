//go:build !ditto

package main

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestScriptsNoDitto covers what a build without the ditto tag says when the
// ditto backend is requested.
func TestScriptsNoDitto(t *testing.T) {
	testscript.Run(t, scriptParams("testdata/script-noditto"))
}

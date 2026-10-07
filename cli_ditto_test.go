//go:build ditto

package main

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestScriptsDitto drives the CLI against Ditto's local store. Sync is never
// started against a server, so the URL is a placeholder and nothing leaves the
// machine. It needs libdittoffi on the linker and loader paths.
func TestScriptsDitto(t *testing.T) {
	testscript.Run(t, scriptParams("testdata/script-ditto",
		"SKATE_BACKEND=ditto",
		"SKATE_DITTO_DATABASE_ID=00000000-0000-4000-8000-000000000000",
		"SKATE_DITTO_URL=https://example.invalid",
	))
}

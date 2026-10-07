//go:build ditto

package main

import (
	"testing"

	"github.com/getditto/ditto-go-sdk/v5/ditto"
)

// TestDittoKV runs the shared contract against Ditto's real local store.
// Nothing here talks to the network: sync is never started, so the URL is a
// placeholder. It needs libdittoffi on the linker and loader paths.
//
// Ditto aborts the process if one is opened twice, so both dbs share a single
// instance.
func TestDittoKV(t *testing.T) {
	storePath = t.TempDir()
	t.Cleanup(func() { storePath = "" })
	t.Setenv(envDittoDatabaseID, "00000000-0000-4000-8000-000000000000")
	t.Setenv(envDittoURL, "https://example.invalid")

	s, err := dittoSettingsFromEnv(false)
	if err != nil {
		t.Fatal(err)
	}
	var d *ditto.Ditto
	d, err = openDitto(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)

	testKVContract(t, func(db string) kv { return dittoKV{d: d, db: db} })

	names, err := dittoNames(d)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(names, []string{"a", "other"}) {
		t.Fatalf("dittoNames = %q, want [a other]", names)
	}
}

func TestDittoSettingsFromEnv(t *testing.T) {
	t.Setenv(envDittoDatabaseID, "id")
	t.Setenv(envDittoURL, "")
	if _, err := dittoSettingsFromEnv(false); err == nil {
		t.Error("want an error when the URL is unset")
	}
	t.Setenv(envDittoURL, "https://x")
	t.Setenv(envDittoToken, "")
	if _, err := dittoSettingsFromEnv(false); err != nil {
		t.Errorf("token must not be required for local commands: %v", err)
	}
	if _, err := dittoSettingsFromEnv(true); err == nil {
		t.Error("want an error when syncing without a token")
	}
}

func TestDittoDocRoundTrip(t *testing.T) {
	for _, v := range [][]byte{[]byte("text"), {}, {0x00, 0xff, 0xfe}} {
		k, got, err := decodeDittoDoc(encodeDittoDoc("k@d", "d", []byte("k"), v))
		if err != nil || string(k) != "k" || string(got) != string(v) {
			t.Errorf("round trip of %x: key=%q value=%x err=%v", v, k, got, err)
		}
	}
}

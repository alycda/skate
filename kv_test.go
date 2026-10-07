package main

import (
	"bytes"
	"errors"
	"testing"
)

// testKVContract is the behavior every backend must share. open returns the
// kv for a db name; kvs opened for different names must be isolated from each
// other. Close must be safe to call more than once.
func testKVContract(t *testing.T, open func(db string) kv) {
	t.Helper()
	a, other := open("a"), open("other")
	t.Cleanup(func() { _ = a.Close(); _ = other.Close() })

	if _, err := a.Get([]byte("missing")); !errors.Is(err, errKeyNotFound) {
		t.Fatalf("Get(missing) = %v, want errKeyNotFound", err)
	}

	for _, kvp := range []struct{ k, v string }{{"b", "2"}, {"a", "1"}, {"c", "3"}, {"kitty litter", "smells great"}} {
		if err := a.Set([]byte(kvp.k), []byte(kvp.v)); err != nil {
			t.Fatalf("Set(%q): %v", kvp.k, err)
		}
	}
	got, err := a.Get([]byte("kitty litter"))
	if err != nil || string(got) != "smells great" {
		t.Fatalf("Get(kitty litter) = %q, %v", got, err)
	}

	if err := a.Set([]byte("a"), []byte("one")); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Get([]byte("a")); string(got) != "one" {
		t.Fatalf("overwrite: Get(a) = %q, want one", got)
	}

	scan := func(opts scanOptions) (keys, values []string) {
		t.Helper()
		err := a.Scan(opts, func(k, v []byte) error {
			keys = append(keys, string(k))
			values = append(values, string(v))
			return nil
		})
		if err != nil {
			t.Fatalf("Scan(%+v): %v", opts, err)
		}
		return keys, values
	}
	keys, values := scan(scanOptions{})
	wantKeys := []string{"a", "b", "c", "kitty litter"}
	if !equal(keys, wantKeys) || !equal(values, []string{"one", "2", "3", "smells great"}) {
		t.Fatalf("Scan = %q %q", keys, values)
	}
	keys, _ = scan(scanOptions{reverse: true})
	if !equal(keys, []string{"kitty litter", "c", "b", "a"}) {
		t.Fatalf("Scan(reverse) keys = %q", keys)
	}
	keys, values = scan(scanOptions{keysOnly: true})
	if !equal(keys, wantKeys) {
		t.Fatalf("Scan(keysOnly) keys = %q", keys)
	}
	for _, v := range values {
		if v != "" {
			t.Fatalf("Scan(keysOnly) passed value %q, want none", v)
		}
	}

	// Binary and empty values round-trip byte for byte.
	bin := []byte{0x00, 0xff, 0xfe, 'b', 'i', 'n'}
	for k, v := range map[string][]byte{"bin": bin, "empty": {}} {
		if err := a.Set([]byte(k), v); err != nil {
			t.Fatalf("Set(%s): %v", k, err)
		}
		got, err := a.Get([]byte(k))
		if err != nil || !bytes.Equal(got, v) {
			t.Fatalf("Get(%s) = %x, %v; want %x", k, got, err, v)
		}
	}

	// Other dbs don't see a's keys, and vice versa.
	if _, err := other.Get([]byte("a")); !errors.Is(err, errKeyNotFound) {
		t.Fatalf("other.Get(a) = %v, want errKeyNotFound", err)
	}
	if err := other.Set([]byte("a"), []byte("other-a")); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Get([]byte("a")); string(got) != "one" {
		t.Fatalf("a.Get(a) = %q after other.Set, want one", got)
	}
	n := 0
	_ = other.Scan(scanOptions{}, func(_, _ []byte) error { n++; return nil })
	if n != 1 {
		t.Fatalf("other has %d pairs, want 1", n)
	}

	// Delete removes the key, and deleting a missing key is not an error.
	if err := a.Delete([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get([]byte("b")); !errors.Is(err, errKeyNotFound) {
		t.Fatalf("Get(b) after Delete = %v, want errKeyNotFound", err)
	}
	if err := a.Delete([]byte("never-existed")); err != nil {
		t.Fatalf("Delete(missing) = %v, want nil", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBadgerKV(t *testing.T) {
	storePath = t.TempDir()
	t.Cleanup(func() { storePath = "" })
	testKVContract(t, func(db string) kv {
		t.Helper()
		k, err := openBadgerKV(db)
		if err != nil {
			t.Fatal(err)
		}
		return k
	})
}

func TestBackendName(t *testing.T) {
	t.Cleanup(func() { backendFlag = "" })
	for _, tc := range []struct {
		flag, env, want string
		wantErr         bool
	}{
		{"", "", backendBadger, false},
		{"", "ditto", backendDitto, false},
		{"badger", "ditto", backendBadger, false}, // flag beats env
		{"ditto", "", backendDitto, false},
		{"nope", "", "", true},
		{"", "nope", "", true},
	} {
		backendFlag = tc.flag
		t.Setenv("SKATE_BACKEND", tc.env)
		got, err := backendName()
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("flag=%q env=%q: got %q, %v", tc.flag, tc.env, got, err)
		}
	}
}

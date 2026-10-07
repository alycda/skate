package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

const (
	backendBadger = "badger"
	backendDitto  = "ditto"
)

// dittoDirPrefix marks directories in the store that hold Ditto persistence
// rather than a badger database, so listing databases can skip them.
const dittoDirPrefix = ".ditto-"

func isDittoDir(name string) bool {
	return strings.HasPrefix(name, dittoDirPrefix)
}

// errKeyNotFound is returned by every backend when a key does not exist. It
// aliases badger's sentinel so the user-visible message stays what it was
// before backends existed.
var errKeyNotFound = badger.ErrKeyNotFound

// scanOptions control the order and shape of kv.Scan.
type scanOptions struct {
	reverse  bool
	keysOnly bool
}

// kv is the storage a single @db resolves to.
type kv interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	// Scan calls fn for every pair in lexicographic key order. When keysOnly
	// is set, value is nil. The slices are only valid for the duration of fn.
	Scan(opts scanOptions, fn func(key, value []byte) error) error
	Close() error
}

// backendName resolves the selected backend: --backend, then SKATE_BACKEND,
// then badger.
func backendName() (string, error) {
	name := backendFlag
	if name == "" {
		name = os.Getenv("SKATE_BACKEND")
	}
	switch name {
	case "":
		return backendBadger, nil
	case backendBadger, backendDitto:
		return name, nil
	default:
		return "", fmt.Errorf("unknown backend %q, use %q or %q", name, backendBadger, backendDitto)
	}
}

func openKV(name string) (kv, error) {
	if name == "" {
		name = "default"
	}
	b, err := backendName()
	if err != nil {
		return nil, err
	}
	if b == backendDitto {
		return openDittoKV(name)
	}
	return openBadgerKV(name)
}

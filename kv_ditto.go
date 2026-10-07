//go:build ditto

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/getditto/ditto-go-sdk/v5/ditto"
	"github.com/spf13/cobra"
)

// The ditto backend keeps every skate database in one Ditto collection. A
// pair is one document:
//
//	_id    "KEY@DB". Neither part can contain "@" (keyParser rejects it), so
//	       the id is unambiguous and matches the CLI's own KEY@DB syntax.
//	db     the @DB name
//	key    the key
//	value  the value; base64 when binary is true
//	binary true when value is not valid UTF-8
//
// Writes land in Ditto's local store and replicate while `skate sync` runs.
const dittoCollection = "skate_kv"

const (
	envDittoDatabaseID = "SKATE_DITTO_DATABASE_ID"
	envDittoURL        = "SKATE_DITTO_URL"
	envDittoToken      = "SKATE_DITTO_TOKEN" //nolint:gosec // the name of the variable, not a credential
	envDittoLogLevel   = "SKATE_DITTO_LOG_LEVEL"
)

// setDittoLogLevel quiets the SDK, which logs to stderr at info level and
// would bury every command's output. SKATE_DITTO_LOG_LEVEL (error, warning,
// info, debug, verbose) turns it back up for troubleshooting sync.
func setDittoLogLevel() error {
	levels := map[string]ditto.LogLevel{
		"error":   ditto.LogLevelError,
		"warning": ditto.LogLevelWarning,
		"info":    ditto.LogLevelInfo,
		"debug":   ditto.LogLevelDebug,
		"verbose": ditto.LogLevelVerbose,
	}
	name := strings.ToLower(os.Getenv(envDittoLogLevel))
	if name == "" {
		name = "error"
	}
	level, ok := levels[name]
	if !ok {
		return fmt.Errorf("%s: unknown level %q, use error, warning, info, debug or verbose", envDittoLogLevel, name)
	}
	ditto.SetMinimumLogLevel(level)
	return nil
}

type dittoSettings struct {
	databaseID string
	url        string
	token      string
}

// dittoSettingsFromEnv reads the connection settings. The token is only
// needed to sync, so it is required only when needToken is set.
func dittoSettingsFromEnv(needToken bool) (dittoSettings, error) {
	s := dittoSettings{
		databaseID: os.Getenv(envDittoDatabaseID),
		url:        os.Getenv(envDittoURL),
		token:      os.Getenv(envDittoToken),
	}
	required := []struct{ name, val string }{
		{envDittoDatabaseID, s.databaseID},
		{envDittoURL, s.url},
	}
	if needToken {
		required = append(required, struct{ name, val string }{envDittoToken, s.token})
	}
	for _, r := range required {
		if r.val == "" {
			return s, fmt.Errorf("the ditto backend needs %s to be set", r.name)
		}
	}
	return s, nil
}

func openDitto(s dittoSettings) (*ditto.Ditto, error) {
	if err := setDittoLogLevel(); err != nil {
		return nil, err
	}
	dir, err := getFilePath(dittoDirPrefix + s.databaseID)
	if err != nil {
		return nil, err
	}
	// WithPersistenceDirectory resolves relative paths against Ditto's own
	// root, not the working directory.
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve ditto directory: %w", err)
	}
	config := ditto.DefaultDittoConfig().
		WithDatabaseID(s.databaseID).
		WithConnect(&ditto.DittoConfigConnectServer{URL: s.url}).
		WithPersistenceDirectory(dir)
	d, err := ditto.Open(config)
	if err != nil {
		return nil, fmt.Errorf("open ditto: %w", err)
	}
	return d, nil
}

type dittoKV struct {
	d  *ditto.Ditto
	db string
}

func openDittoKV(db string) (kv, error) {
	s, err := dittoSettingsFromEnv(false)
	if err != nil {
		return nil, err
	}
	d, err := openDitto(s)
	if err != nil {
		return nil, err
	}
	return dittoKV{d: d, db: db}, nil
}

func (k dittoKV) id(key []byte) (string, error) {
	if !utf8.Valid(key) {
		return "", errors.New("the ditto backend requires keys to be valid UTF-8")
	}
	return string(key) + "@" + k.db, nil
}

func (k dittoKV) Get(key []byte) ([]byte, error) {
	id, err := k.id(key)
	if err != nil {
		return nil, err
	}
	res, err := k.d.Store().Execute(
		"SELECT * FROM "+dittoCollection+" WHERE _id = :id",
		map[string]any{"id": id},
	)
	if err != nil {
		return nil, fmt.Errorf("ditto get: %w", err)
	}
	defer res.Close()
	if res.ItemCount() == 0 {
		return nil, errKeyNotFound
	}
	_, v, err := decodeDittoDoc(res.Item(0).Value())
	return v, err
}

func (k dittoKV) Set(key, value []byte) error {
	id, err := k.id(key)
	if err != nil {
		return err
	}
	doc := encodeDittoDoc(id, k.db, key, value)
	res, err := k.d.Store().Execute(
		"INSERT INTO "+dittoCollection+" DOCUMENTS (:doc) ON ID CONFLICT DO UPDATE",
		map[string]any{"doc": doc},
	)
	if err != nil {
		return fmt.Errorf("ditto set: %w", err)
	}
	res.Close()
	return nil
}

func (k dittoKV) Delete(key []byte) error {
	id, err := k.id(key)
	if err != nil {
		return err
	}
	res, err := k.d.Store().Execute(
		"DELETE FROM "+dittoCollection+" WHERE _id = :id",
		map[string]any{"id": id},
	)
	if err != nil {
		return fmt.Errorf("ditto delete: %w", err)
	}
	res.Close()
	return nil
}

type dittoPair struct{ key, value []byte }

// Scan reads the whole database and sorts it here: bytewise key order is
// what the badger backend gives, and I have not verified that DQL's ORDER BY
// collates strings the same way.
func (k dittoKV) Scan(opts scanOptions, fn func(key, value []byte) error) error {
	res, err := k.d.Store().Execute(
		"SELECT * FROM "+dittoCollection+" WHERE db = :db",
		map[string]any{"db": k.db},
	)
	if err != nil {
		return fmt.Errorf("ditto list: %w", err)
	}
	defer res.Close()

	pairs := make([]dittoPair, 0, res.ItemCount())
	for _, item := range res.Items() {
		key, value, err := decodeDittoDoc(item.Value())
		if err != nil {
			return err
		}
		pairs = append(pairs, dittoPair{key: key, value: value})
	}
	slices.SortFunc(pairs, func(a, b dittoPair) int { return bytes.Compare(a.key, b.key) })
	if opts.reverse {
		slices.Reverse(pairs)
	}
	for _, p := range pairs {
		v := p.value
		if opts.keysOnly {
			v = nil
		}
		if err := fn(p.key, v); err != nil {
			return err
		}
	}
	return nil
}

func (k dittoKV) Close() error {
	k.d.Close()
	return nil
}

func encodeDittoDoc(id, db string, key, value []byte) ditto.Document {
	doc := ditto.Document{"_id": id, "db": db, "key": string(key)}
	if utf8.Valid(value) {
		doc["value"] = string(value)
		doc["binary"] = false
	} else {
		doc["value"] = base64.StdEncoding.EncodeToString(value)
		doc["binary"] = true
	}
	return doc
}

func decodeDittoDoc(doc ditto.Document) (key, value []byte, err error) {
	k, ok := doc["key"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("ditto document %v has no string key", doc["_id"])
	}
	v, ok := doc["value"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("ditto document %v has no string value", doc["_id"])
	}
	if bin, _ := doc["binary"].(bool); bin {
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, nil, fmt.Errorf("ditto document %v: decode binary value: %w", doc["_id"], err)
		}
		return []byte(k), raw, nil
	}
	return []byte(k), []byte(v), nil
}

// dittoNames returns the sorted names of the databases that have at least
// one pair in d.
func dittoNames(d *ditto.Ditto) ([]string, error) {
	res, err := d.Store().Execute("SELECT db FROM " + dittoCollection)
	if err != nil {
		return nil, fmt.Errorf("ditto list-dbs: %w", err)
	}
	defer res.Close()
	seen := map[string]struct{}{}
	for _, item := range res.Items() {
		if name, ok := item.Value()["db"].(string); ok {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

func dittoDbs() ([]string, error) {
	s, err := dittoSettingsFromEnv(false)
	if err != nil {
		return nil, err
	}
	d, err := openDitto(s)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return dittoNames(d)
}

// deleteDittoDb opens Ditto once: a second Open in the same process, even
// after Close, fails on Ditto's directory lock and aborts the process.
func deleteDittoDb(arg string) error {
	name, err := nameFromArgs([]string{arg})
	if err != nil {
		return err
	}
	s, err := dittoSettingsFromEnv(false)
	if err != nil {
		return err
	}
	d, err := openDitto(s)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := dittoNames(d)
	if err != nil {
		return err
	}
	if !slices.Contains(names, name) {
		return fmt.Errorf("%q does not exist, %w", arg, errDBNotFound{suggestions: suggestDbs(arg, formatDbs(names))})
	}
	target := "@" + name + " (ditto: also deleted on every peer it syncs with)"
	confirmed, err := confirmDelete(target)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintf(os.Stderr, "Did not delete %q\n", "@"+name)
		return nil
	}
	res, err := d.Store().Execute(
		"DELETE FROM "+dittoCollection+" WHERE db = :db",
		map[string]any{"db": name},
	)
	if err != nil {
		return fmt.Errorf("ditto delete-db: %w", err)
	}
	res.Close()
	fmt.Fprintf(os.Stderr, "Deleted %q\n", "@"+name)
	return nil
}

// syncDitto replicates the skate collection with Ditto's peers until the
// process is interrupted or --timeout elapses. Ditto owns its persistence
// directory, so other skate commands on the ditto backend cannot run
// meanwhile.
func syncDitto(cmd *cobra.Command, _ []string) error {
	b, err := backendName()
	if err != nil {
		return err
	}
	if b != backendDitto {
		return fmt.Errorf("sync needs the ditto backend: pass --backend ditto or set SKATE_BACKEND=ditto")
	}
	s, err := dittoSettingsFromEnv(true)
	if err != nil {
		return err
	}
	d, err := openDitto(s)
	if err != nil {
		return err
	}
	defer d.Close()

	// Required before Sync().Start() on a server connection.
	d.Auth().SetExpirationHandler(func(d *ditto.Ditto, _ time.Duration) {
		if _, err := d.Auth().Login(s.token, ditto.DevelopmentAuthenticationProvider()); err != nil {
			fmt.Fprintf(os.Stderr, "ditto: authentication failed: %v\n", err)
		}
	})
	// Without a subscription this peer would push its writes but never pull
	// anyone else's.
	sub, err := d.Sync().RegisterSubscription("SELECT * FROM " + dittoCollection)
	if err != nil {
		return fmt.Errorf("ditto subscribe: %w", err)
	}
	defer sub.Cancel()
	if err := d.Sync().Start(); err != nil {
		return fmt.Errorf("ditto start sync: %w", err)
	}
	defer d.Sync().Stop()

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if syncTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, syncTimeout)
		defer cancel()
	}
	fmt.Fprintln(os.Stderr, "Syncing; press Ctrl-C to stop.")
	<-ctx.Done()
	return nil
}

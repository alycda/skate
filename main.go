// Package main provides the skate CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agnivade/levenshtein"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	gap "github.com/muesli/go-app-paths"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	reverseIterate   bool
	keysIterate      bool
	valuesIterate    bool
	showBinary       bool
	delimiterIterate string
	copyToClipboard  bool
	storePath        string
	backendFlag      string
	syncTimeout      time.Duration

	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Bold(true)

	rootCmd = &cobra.Command{
		Use:   "skate",
		Short: "Skate, a personal key value store.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	setCmd = &cobra.Command{
		Use:     "set KEY[@DB] [VALUE]",
		Short:   "Set a value for a key with an optional @ db. If VALUE is omitted, read value from the standard input.",
		Example: "  skate set foo bar\n  skate set foo <./bar.txt",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    set,
	}

	getCmd = &cobra.Command{
		Use:           "get KEY[@DB]",
		Short:         "Get a value for a key with an optional @ db.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE:          get,
	}

	deleteCmd = &cobra.Command{
		Use:     "delete KEY[@DB]",
		Short:   "Delete a key with an optional @ db.",
		Aliases: []string{"del", "rm"},
		Args:    cobra.ExactArgs(1),
		RunE:    del,
	}

	listCmd = &cobra.Command{
		Use:     "list [@DB]",
		Short:   "List key value pairs with an optional @ db.",
		Aliases: []string{"ls"},
		Args:    cobra.MaximumNArgs(1),
		RunE:    list,
	}

	listDbsCmd = &cobra.Command{
		Use:     "list-dbs",
		Short:   "List databases.",
		Aliases: []string{"ls-db"},
		Args:    cobra.NoArgs,
		RunE:    listDbs,
	}

	syncCmd = &cobra.Command{
		Use:   "sync",
		Short: "Sync the ditto backend with its peers until interrupted or --timeout elapses.",
		Args:  cobra.NoArgs,
		RunE:  syncDitto,
	}

	deleteDbCmd = &cobra.Command{
		Use:     "delete-db [@DB]",
		Hidden:  false,
		Short:   "Delete a database",
		Aliases: []string{"del-db", "rm-db"},
		Args:    cobra.MinimumNArgs(1),
		RunE:    deleteDb,
	}
)

type errDBNotFound struct {
	suggestions []string
}

func (err errDBNotFound) Error() string {
	if len(err.suggestions) == 0 {
		return "no suggestions found"
	}
	return fmt.Sprintf("did you mean %q", strings.Join(err.suggestions, ", "))
}

//nolint:wrapcheck
func set(cmd *cobra.Command, args []string) error {
	k, n, err := keyParser(args[0])
	if err != nil {
		return err
	}
	db, err := openKV(n)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	if len(args) == 2 {
		return db.Set(k, []byte(args[1]))
	}
	bts, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return err
	}
	return db.Set(k, bts)
}

//nolint:wrapcheck
func get(_ *cobra.Command, args []string) error {
	k, n, err := keyParser(args[0])
	if err != nil {
		return err
	}
	db, err := openKV(n)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	v, err := db.Get(k)
	if err != nil {
		return err
	}
	printFromKV("%s", v)
	if copyToClipboard {
		return clipboard.WriteAll(string(v))
	}
	return nil
}

//nolint:wrapcheck
func del(_ *cobra.Command, args []string) error {
	k, n, err := keyParser(args[0])
	if err != nil {
		return err
	}
	db, err := openKV(n)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	return db.Delete(k)
}

// TODO: use lists/tables/trees for this?
func listDbs(*cobra.Command, []string) error {
	dbs, err := getDbs()
	for _, db := range dbs {
		fmt.Println(db)
	}
	return err
}

// getDbs: returns a formatted list of available Skate DBs.
//
//nolint:wrapcheck
func getDbs() ([]string, error) {
	b, err := backendName()
	if err != nil {
		return nil, err
	}
	if b == backendDitto {
		names, err := dittoDbs()
		return formatDbs(names), err
	}
	filepath, err := getFilePath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath)
	if err != nil {
		return nil, err
	}
	var dbList []string
	for _, e := range entries {
		if e.IsDir() && !isDittoDir(e.Name()) {
			dbList = append(dbList, e.Name())
		}
	}
	return formatDbs(dbList), nil
}

func formatDbs(dbs []string) []string {
	out := make([]string, 0, len(dbs))
	for _, db := range dbs {
		out = append(out, "@"+db)
	}
	return out
}

// getFilePath: get the file path to the skate databases.
//
//nolint:wrapcheck
func getFilePath(args ...string) (string, error) {
	dir := storePath
	if dir == "" {
		dir = os.Getenv("SKATE_STORE")
	}
	if dir == "" {
		scope := gap.NewScope(gap.User, "charm")
		dd, pathErr := scope.DataPath("")
		if pathErr != nil {
			return "", pathErr
		}
		dir = filepath.Join(dd, "kv")
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, args...)...), nil
}

// deleteDb: delete a Skate database.
//
//nolint:wrapcheck
func deleteDb(_ *cobra.Command, args []string) error {
	b, err := backendName()
	if err != nil {
		return err
	}
	if b == backendDitto {
		return deleteDittoDb(args[0])
	}
	path, err := findDb(args[0])
	var errNotFound errDBNotFound
	if errors.As(err, &errNotFound) {
		fmt.Fprintf(os.Stderr, "%q does not exist, %s\n", args[0], err.Error())
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error: %s", err.Error())
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	showpath := path
	if err == nil && strings.HasPrefix(path, home) {
		showpath = filepath.Join("~", strings.TrimPrefix(showpath, home))
	}
	confirmed, err := confirmDelete(showpath)
	if err != nil {
		return err
	}
	if confirmed {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleted %q\n", showpath)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Did not delete %q\n", showpath)
	return nil
}

// confirmDelete asks on stdin whether target and all its contents may be
// deleted.
//
//nolint:wrapcheck
func confirmDelete(target string) (bool, error) {
	var confirmation string
	message := fmt.Sprintf("Are you sure you want to delete '%s' and all its contents? (y/n)", warningStyle.Render(target))
	message = lipgloss.NewStyle().Width(78).Render(message)
	fmt.Println(message)

	// TODO: use huh
	if _, err := fmt.Scanln(&confirmation); err != nil {
		return false, err
	}
	return confirmation == "y", nil
}

// findDb: returns the path to the named db or an errDBNotFound if no
// match is found.
func findDb(name string) (string, error) {
	sName, err := nameFromArgs([]string{name})
	if err != nil {
		return "", err
	}
	path, err := getFilePath(sName)
	if err != nil {
		return "", err
	}
	_, err = os.Stat(path)
	if sName == "" || os.IsNotExist(err) {
		dbs, err := getDbs()
		if err != nil {
			return "", err
		}
		return "", errDBNotFound{suggestions: suggestDbs(name, dbs)}
	}
	return path, nil
}

// suggestDbs: returns the dbs close enough to name to be a likely typo.
func suggestDbs(name string, dbs []string) []string {
	var suggestions []string
	for _, db := range dbs {
		diff := int(math.Abs(float64(len(db) - len(name))))
		levenshteinDistance := levenshtein.ComputeDistance(name, db)
		suggestByLevenshtein := levenshteinDistance <= diff
		if suggestByLevenshtein {
			suggestions = append(suggestions, db)
		}
	}
	return suggestions
}

//nolint:wrapcheck
func list(_ *cobra.Command, args []string) error {
	var k string
	var pf string
	if keysIterate || valuesIterate {
		pf = "%s\n"
	} else {
		var err error
		pf, err = strconv.Unquote(fmt.Sprintf(`"%%s%s%%s\n"`, delimiterIterate))
		if err != nil {
			return err
		}
	}
	if len(args) == 1 {
		k = args[0]
	}
	_, n, err := keyParser(k)
	if err != nil {
		return err
	}
	db, err := openKV(n)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	return db.Scan(scanOptions{reverse: reverseIterate, keysOnly: keysIterate}, func(k, v []byte) error {
		switch {
		case keysIterate:
			printFromKV(pf, k)
		case valuesIterate:
			printFromKV(pf, v)
		default:
			printFromKV(pf, k, v)
		}
		return nil
	})
}

func nameFromArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	_, n, err := keyParser(args[0])
	if err != nil {
		return "", err
	}
	return n, nil
}

func printFromKV(pf string, vs ...[]byte) {
	nb := "(omitted binary data)"
	fvs := make([]any, 0)
	isatty := term.IsTerminal(int(os.Stdin.Fd()))
	for _, v := range vs {
		if isatty && !showBinary && !utf8.Valid(v) {
			fvs = append(fvs, nb)
		} else {
			fvs = append(fvs, string(v))
		}
	}
	fmt.Printf(pf, fvs...)
	if isatty && !strings.HasSuffix(pf, "\n") {
		fmt.Println()
	}
}

func keyParser(k string) ([]byte, string, error) {
	var key, db string
	ps := strings.Split(k, "@")
	switch len(ps) {
	case 1:
		key = strings.ToLower(ps[0])
	case 2:
		key = strings.ToLower(ps[0])
		db = strings.ToLower(ps[1])
	default:
		return nil, "", fmt.Errorf("bad key format, use KEY@DB")
	}
	return []byte(key), db, nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&storePath, "store", "", "path to the Skate store (defaults to SKATE_STORE or the user data directory)")
	rootCmd.PersistentFlags().StringVar(&backendFlag, "backend", "", "storage backend: badger or ditto (defaults to SKATE_BACKEND, then badger)")
	syncCmd.Flags().DurationVar(&syncTimeout, "timeout", 0, "stop syncing after this long (default: run until interrupted)")

	listCmd.Flags().BoolVarP(&reverseIterate, "reverse", "r", false, "list in reverse lexicographic order")
	listCmd.Flags().BoolVarP(&keysIterate, "keys-only", "k", false, "only print keys and don't fetch values from the db")
	listCmd.Flags().BoolVarP(&valuesIterate, "values-only", "v", false, "only print values")
	listCmd.Flags().StringVarP(&delimiterIterate, "delimiter", "d", "\t", "delimiter to separate keys and values")
	listCmd.Flags().BoolVarP(&showBinary, "show-binary", "b", false, "print binary values")
	getCmd.Flags().BoolVarP(&showBinary, "show-binary", "b", false, "print binary values")
	getCmd.Flags().BoolVarP(&copyToClipboard, "copy", "c", false, "copy value to clipboard")

	rootCmd.AddCommand(
		getCmd,
		setCmd,
		deleteCmd,
		listCmd,
		listDbsCmd,
		deleteDbCmd,
		syncCmd,
	)
}

func main() {
	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

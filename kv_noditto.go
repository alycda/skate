//go:build !ditto

package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNoDitto is returned when the ditto backend is selected in a build that
// was not compiled with the ditto tag.
var errNoDitto = errors.New("this build of skate does not include the ditto backend; rebuild with -tags ditto (see README)")

// Builds without the ditto tag carry no Ditto SDK or native library, so the
// ditto backend is a stub that explains how to get a build that has it.

func openDittoKV(string) (kv, error) { return nil, errNoDitto }

func dittoDbs() ([]string, error) { return nil, errNoDitto }

func deleteDittoDb(string) error { return errNoDitto }

func syncDitto(*cobra.Command, []string) error { return errNoDitto }

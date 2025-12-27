// Package main is the entry point of the commandline tool rowx for migrating a
// database to its next schema state and to generate Go structures, mapping to
// database tables from an existing database.
// Copyright (c) 2025 Красимир Беров
package main

import (
	"os"

	"github.com/kberov/rowx/rx"
)

func init() {
	output = os.Stderr
	_init()
}

func main() {
	i := run()
	if e := rx.DB().Close(); e != nil {
		rx.Logger.Errorf("%w", e)
	}
	os.Exit(i)
}

package main

import (
	"bytes"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kberov/rowx/rx"
	"github.com/stretchr/testify/require"
)

//nolint:gosec // G404
var tempDBFile = os.TempDir() + `/rowx_test` + strconv.Itoa(rand.Intn(999)) + `.sqlite`
var cases = []struct {
	args   []string
	code   int
	output string
	setup  func(t *testing.T)
}{
	{
		args:   []string{},
		code:   0,
		output: "\nUSAGE:",
	},
	{
		args:   []string{`help`},
		code:   0,
		output: "\nUSAGE:",
	},
	{
		args:   []string{`-help`},
		code:   0,
		output: "\nUSAGE:",
	},
	{
		args:   []string{`migrate`},
		code:   1,
		output: "All flags beside",
	},
	{
		args:   []string{`migrate`, `help`},
		code:   1,
		output: "All flags beside",
	},
	{
		args:   []string{`migrate`, `-what`},
		code:   1,
		output: "flag provided but not defined: -what",
	},
	{
		args:   []string{`migrate`, `-log_level`, `UNKNOWN`},
		code:   1,
		output: "No such log_level: UNKNOWN.\n",
	},
	{
		args: []string{`migrate`, `-sql_file`, `rx/testdata/migrations_01.sql`,
			`-dsn`, tempDBFile, `-direction`, `left`},
		code:   2,
		output: "direction can be only",
	},
	{
		args: []string{`migrate`, `-sql_file`, `rx/testdata/migrations_01.sql`,
			`-dsn`, tempDBFile, `-direction`, `up`},
		code:   0,
		output: "Applying 201804092200 up",
	},
	{
		args:   []string{`generate`},
		code:   1,
		output: "are mandatory!\n",
	},
	{
		args:   []string{`generate`, `help`},
		code:   1,
		output: "are mandatory!\n",
	},
	{
		args:   []string{`generate`, `-what`},
		code:   1,
		output: "flag provided but not defined: -what\n  generate",
	},
	{
		args:   []string{`generate`, `-log_level`, `UNKNOWN`},
		code:   1,
		output: "No such log_level: UNKNOWN.\n",
	},
	{
		args:   []string{`generate`, `-dsn`, tempDBFile, `-package`, `rx/testdata/example/model`},
		code:   0,
		output: "_structs.go...",
		setup: func(t *testing.T) {
			err := os.MkdirAll(`rx/testdata/example/model`, 0750)
			require.NoErrorf(t, err, `Unexpected error: %+v`, err)
		},
	},
	{
		args:   []string{`alabalanica`},
		code:   1,
		output: "\nUknown action ",
	},
}

func TestRun(t *testing.T) {
	osArgs := os.Args
	output = bytes.NewBufferString("")
	osStderr := os.Stderr
	defer func() {
		os.Stderr = osStderr
		os.Args = osArgs
	}()

	for _, tc := range cases {
		// reset os.Args
		os.Args = []string{`./rowx`}
		// Reset flags.
		_init()
		name := strings.Join(tc.args, `_`)
		output.(*bytes.Buffer).Reset()
		os.Args = append(os.Args, tc.args...)
		rx.Logger.SetOutput(output)
		if tc.setup != nil {
			tc.setup(t)
		}
		t.Run(name, func(t *testing.T) {
			code := run()
			require.Equalf(t, tc.code, code,
				`Expected exit code was %d, but we got %d.`, tc.code, code)
			require.Containsf(t, output.(*bytes.Buffer).String(), tc.output,
				`Expected output to contain [%s], but it is: [%s]`, tc.output, output)
		})
	}
}

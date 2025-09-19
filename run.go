package main

import (
	"bytes"
	"flag"
	"io"
	"os"

	"github.com/labstack/gommon/log"
	"github.com/valyala/fasttemplate"

	"github.com/kberov/rowx/rx"
)

const (
	migrate  string = `migrate`
	generate string = `generate`
)

var (
	mFlags, gFlags      *flag.FlagSet
	dsn, sqlFilePath    string
	direction, logLevel string
	packagePath, action string
	output              io.Writer
	logLevels           = map[string]log.Lvl{"DEBUG": 1, "INFO": 2, "WARN": 3, "ERROR": 4, "OFF": 5}
)

func init() {
	_init()
}

func _init() {
	flag.CommandLine.SetOutput(output)
	flag.Usage = usage
	mFlags = flag.NewFlagSet(migrate, flag.ContinueOnError)
	mFlags.SetOutput(output)
	mFlags.StringVar(&dsn, `dsn`, ``, `Database to connect to.`)
	mFlags.StringVar(&sqlFilePath, `sql_file`, ``, `Path to sql file for migration.`)
	mFlags.StringVar(&direction, `direction`, ``, `Direction for migration: up or down.`)
	mFlags.StringVar(&logLevel, `log_level`, `INFO`,
		`One of DEBUG, INFO, WARN, ERROR, OFF. Default is INFO.`)
	mFlags.Usage = func() {
		say(migrateTmpl, output, rx.Map{
			migrate:          mFlags.Name(),
			`sql_file_help`:  mFlags.Lookup(`sql_file`).Usage,
			`mdsn_help`:      mFlags.Lookup(`dsn`).Usage,
			`direction_help`: mFlags.Lookup(`direction`).Usage,
			`ll_help`:        mFlags.Lookup(`log_level`).Usage,
		})
	}

	gFlags = flag.NewFlagSet(generate, flag.ContinueOnError)
	gFlags.SetOutput(output)
	mdsn := mFlags.Lookup(`dsn`)
	gFlags.StringVar(&dsn, mdsn.Name, mdsn.DefValue, mdsn.Usage)
	gFlags.StringVar(&packagePath, `package`, ``, "Path to package to generate."+
		" Last folder is the name of\n           the package to be generated.")
	mLogLevel := mFlags.Lookup(`log_level`)
	gFlags.StringVar(&logLevel, mLogLevel.Name, mLogLevel.DefValue, mLogLevel.Usage)
	gFlags.Usage = func() {
		say(generateTmpl, output, rx.Map{
			generate:       gFlags.Name(),
			`package_help`: gFlags.Lookup(`package`).Usage,
			`gdsn_help`:    gFlags.Lookup(`dsn`).Usage,
			`ll_help`:      gFlags.Lookup(`log_level`).Usage,
		})
	}
}

var (
	usageTmpl = `
USAGE: ${exe} "action" flags...

Actions:
  -help, help
    Prints this message and exits.
${migrate}
${generate}
`
	migrateTmpl = `  ${migrate}
  -sql_file  ${sql_file_help}
  -dsn       ${mdsn_help}  
  -direction ${direction_help}
  -log_level ${ll_help}
`
	generateTmpl = `  ${generate}
  -dsn     ${gdsn_help}
  -package ${package_help}
  -log_level ${ll_help}
`
)

func say(tpl string, out io.Writer, _map rx.Map) {
	if _, err := fasttemplate.Execute(tpl, "${", "}", out, _map); err != nil {
		_, _ = out.Write([]byte(err.Error()))
	}
}

func usage() {
	var mFlagsStr bytes.Buffer
	say(migrateTmpl, &mFlagsStr, rx.Map{
		migrate:          mFlags.Name(),
		`sql_file_help`:  mFlags.Lookup(`sql_file`).Usage,
		`mdsn_help`:      mFlags.Lookup(`dsn`).Usage,
		`direction_help`: mFlags.Lookup(`direction`).Usage,
		`ll_help`:        mFlags.Lookup(`log_level`).Usage,
	})
	var gFlagsStr bytes.Buffer
	say(generateTmpl, &gFlagsStr, rx.Map{
		generate:       gFlags.Name(),
		`package_help`: gFlags.Lookup(`package`).Usage,
		`gdsn_help`:    gFlags.Lookup(`dsn`).Usage,
		`ll_help`:      gFlags.Lookup(`log_level`).Usage,
	})
	say(usageTmpl, output, rx.Map{
		`exe`:    os.Args[0],
		migrate:  mFlagsStr.Bytes(),
		generate: gFlagsStr.Bytes(),
	})
}

func run() int {
	if output == nil {
		output = os.Stderr
	}
	if len(os.Args) < 2 {
		usage()
		return 0
	}
	action = os.Args[1]
	switch action {
	case `-help`, `help`:
		flag.Usage()
		return 0
	case migrate:
		return runMigrate()
	case generate:
		return runGenerate()
	default:
		say("\nUknown action '${a}'!\n", output, rx.Map{`a`: action})
		usage()
		return 1
	}
}

func runMigrate() int {
	eh := mFlags.Parse(os.Args[2:])
	if eh != nil {
		return 1
	}

	ll, ok := logLevels[logLevel]
	if !ok {
		say("No such log_level: ${l}.\n", output, rx.Map{`l`: logLevel})
		mFlags.Usage()
		return 1
	}
	rx.Logger.SetLevel(ll)

	if dsn == `` || sqlFilePath == `` || direction == `` {
		say("All flags beside 'log_level' are mandatory!\n", output, rx.Map{})
		mFlags.Usage()
		return 1
	}
	if eh = rx.Migrate(sqlFilePath, dsn, direction); eh != nil {
		rx.Logger.Errorf("\n=====\n%s", eh.Error())
		return 2
	}
	return 0
}

func runGenerate() int {
	eh := gFlags.Parse(os.Args[2:])
	if eh != nil {
		return 1
	}

	ll, ok := logLevels[logLevel]
	if !ok {
		say("No such log_level: ${l}.\n", output, rx.Map{`l`: logLevel})
		gFlags.Usage()
		return 1
	}
	rx.Logger.SetLevel(ll)

	if dsn == `` || packagePath == `` {
		say("'dsn' and 'package' are mandatory!\n", output, rx.Map{})
		gFlags.Usage()
		return 1
	}
	if eh = rx.Generate(dsn, packagePath); eh != nil {
		rx.Logger.Errorf("\n=====\n%s!", eh.Error())
		return 2
	}
	return 0
}

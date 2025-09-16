package rx

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jmoiron/sqlx"
)

func type2str[R Rowx](row R) string {
	return reflect.TypeOf(row).Elem().Name()
}

const type2StrPanicFmtStr = "Could not derive table name from type '%s'!"

/*
TypeToSnake converts struct type name like
AVeryLongAndComplexTableName to 'a_very_long_and_complex_table_name' and
returns it. Panics if the structure is annonimous.
*/
func TypeToSnake[R Rowx](row R) string {
	typestr := type2str(row)
	Logger.Debugf("TypeToSnakeCase typestr: %s", typestr)
	// Anonimous struct
	if typestr == `` {
		Logger.Panicf(type2StrPanicFmtStr, typestr)
	}
	return CamelToSnake(typestr)
}

/*
CamelToSnake is used to convert type names and structure fields to snake
case table columns. We pass it to [reflectx.NewMapperFunc] together with
[ReflectXTag]. For example the string `UserLastFiveComments` is transformed to
`user_last_five_comments`.
*/
func CamelToSnake(text string) string {
	runes := []rune(text)
	if len(runes) == 2 {
		return strings.ToLower(text)
	}
	var snake strings.Builder
	var begins = true
	var wasUpper = true
	for _, r := range runes {
		begins, wasUpper = lowerLetter(&snake, r, begins, wasUpper)
	}
	return snake.String()
}

const connector = '_'

func lowerLetter(snake *strings.Builder, r rune, begins, wasUpper bool) (bool, bool) {
	if unicode.IsUpper(r) && !begins {
		snake.WriteRune(connector)
		snake.WriteRune(unicode.ToLower(r))
		return true, true // begins, wasUpper
	}
	if begins && wasUpper {
		snake.WriteRune(unicode.ToLower(r))
		return false, false // begins, wasUpper
	}
	snake.WriteRune(r)
	return begins, wasUpper
}

/*
SnakeToCamel converts words from snake_case to CamelCase. It will be used to
convert table_name to TableName and column_names to ColumnNames. This will be
done during generation of structures out from tables.
*/
func SnakeToCamel(snake_case_word string) string { //nolint:revive
	runes := []rune(snake_case_word)
	if len(runes) == 2 {
		return strings.ToUpper(snake_case_word)
	}
	splitWords := strings.Split(snake_case_word, `_`)
	for i, word := range splitWords {
		runes := []rune(word)
		if word == `id` {
			splitWords[i] = strings.ToUpper(word)
			continue
		}
		splitWords[i] = strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:]))
	}
	return strings.Join(splitWords, ``)
}

type dir uint8

const (
	up dir = iota
	down
)

func (d dir) String() string {
	return [...]string{`up`, `down`}[d]
}

/*
Migrate executes all not applied schema migrations with the given `direction`,
found in `filePath` and stores in [MigrationsTable] the version, direction and
file path of every applied migration. The migrations comments (headers) are
expected to mach `^--\s*(\d{1,12})\s*(up|down)$`. For example: `--202506092333
up`. All SQL statements must end with semicolumn -- `;`. It is used to split
the migartion into separate statements to be executed. Each migration is
executed in a transaction. The migrations are applied sequentially in the order
they appear in the file.

If the `direction` is `up` the migrations are applied in ascending order.

If the `direction` is `down` the migrations are applied in desscending order.

The explained workflow allows to have more than one migration in the same file
for logically different parts of the application. For example different modules
have their own different migrations but they in some cases have to be applied
in one run - a new release.
*/
func Migrate(filePath, dsn, direction string) error {
	if unknown(direction) {
		return fmt.Errorf(`direction can be only '%s' or '%s'`, up, down)
	}
	/*
		FIXME: dangerous!!! we assume here that DB() was not invoked yet and
		Migrate is called from a main() function. What if it is a called from a
		long-running process? We need another separate singleDB.
	*/
	DSN = dsn
	DB().MustExec(RenderSQLTemplate(`CREATE_MIGRATIONS_TABLE`, SQLMap{`table`: MigrationsTable}))

	migrations, err := parseMigrationFile(filePath)
	if direction == down.String() {
		slices.Reverse(migrations)
	}
	for i, v := range migrations {
		if v.Direction != direction {
			Logger.Debugf(`Skipping not applicable direction %s %s: %s ...`, v.Version, v.Direction, v.Statements.String()[:30])
			continue
		}
		Logger.Debugf(`Applying %d . %s|%s %s ...`,
			i+1, v.Version, v.Direction, v.Statements.String()[:40])

		if err = multiExec(DB(), v.Statements.String()); err != nil {
			return err
		}
		if _, err = NewRx(Migrations{
			Version:   v.Version,
			Direction: v.Direction,
			FilePath:  filePath,
		}).Insert(); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func unknown(direction string) bool {
	return direction != up.String() && direction != down.String()
}

/*
multiExec was stollen from sqlx_test.go and slightly modified as a poor's man
migration. It executes all stementss found in a big multy query string ias one
transaction.
*/
func multiExec(db *sqlx.DB, query string) (err error) {
	stmts := strings.Split(query, ";")
	if len(strings.TrimSpace(stmts[len(stmts)-1])) == 0 {
		stmts = stmts[:len(stmts)-1]
	}
	tx := db.MustBegin()
	// The rollback will be ignored if the tx has been committed already.
	defer func() { _ = tx.Rollback() }()

	for i, s := range stmts {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}
		Logger.Infof("Exec %02d: %s", i+1, s)
		_, err = tx.Exec(s)
		if err != nil {
			return fmt.Errorf(`%s: %s`, err.Error(), s)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return
}

// Migrations is an object, mapped to [MigrationsTable].
type Migrations struct {
	Applied   time.Time `rx:"applied,auto"`
	Version   string
	Direction string
	FilePath  string
}

// Table returns the table for [Migrations].
func (r *Migrations) Table() string {
	return MigrationsTable
}

type migration struct {
	Version    string
	Direction  string
	Statements strings.Builder
}

func parseMigrationFile(filePath string) (migrations []migration, err error) {
	fh, err := safeOpen(filePath)
	if err != nil {
		return migrations, err
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	migrations = make([]migration, 0)
	versionIsApplied := false
	currentVersion := ``
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if version, direction := parseMigrationHeader(line); version != `` && direction != `` {
			_, err = NewRx[Migrations]().Get(
				`version=:ver AND direction =:dir`, SQLMap{`ver`: version, `dir`: direction})
			// If this migration is not found in the applied migrations, we
			// must start collecting its lines to apply it.
			if err != nil && errors.Is(err, sql.ErrNoRows) {
				versionIsApplied = false
				currentVersion = version
				migrations = append(migrations,
					migration{Version: currentVersion, Direction: direction})
			} else if err == nil {
				versionIsApplied = true
			}
			continue
		}
		// Do not collect anything until a header is found or if this verion is
		// already applied.
		if currentVersion == `` || versionIsApplied {
			continue
		}
		// else collect migrations
		migrations[len(migrations)-1].Statements.WriteString(line)
		migrations[len(migrations)-1].Statements.WriteString("\n")
	}
	return migrations, nil
}

func safeOpen(filePath string) (*os.File, error) {
	filePath, _ = filepath.Abs(filepath.Clean(filePath))
	cwd, _ := os.Getwd()
	if !strings.HasPrefix(filePath, cwd) {
		Logger.Panicf(`%s is unsafe. Cannot continue...`, filePath)
	}
	// Logger.Debugf(`Opening a safe path %s`, filePath)
	return os.Open(filePath)
}

var migrationHeader = regexp.MustCompile(`^--\s*(\d{1,12})\s*(up|down)$`)

func parseMigrationHeader(line string) (version, direction string) {
	matches := migrationHeader.FindStringSubmatch(line)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return
}

/*
Generate generates structures for tables, found in database, pointed to by `dsn`
and dumps them to a given `packagePath` directory. Returns an error if
unsuccesfull. The name of the last directory in the path is used as
package name. The directory must exist already.

Two files are created. The first only declares the package and can be modified
by the programmer. It will not be regenerated on subsequent runs. The second
contains all the structures, mapped to tables. It will be regenerated again on
the next run of this function to map the potentially migrated to a new state
schema.
*/
func Generate(dsn string, packagePath string) error {
	DSN = dsn
	dh, err := safeOpen(packagePath)
	if err != nil {
		return err
	}
	defer dh.Close()
	sql := QueryTemplates[`SELECT_TABLE_INFO_sqlite3`].(string)
	info := []columnInfo{}
	if err = DB().Select(&info, sql, MigrationsTable); err != nil {
		return fmt.Errorf(`DB().Select: %w`, err)
	}
	var structsFileString strings.Builder
	dirName := dh.Name()
	preparePackageHeaderForGeneratedStructs(dirName, &structsFileString)
	prepareGeneratedStructs(info, &structsFileString)
	// Logger.Debugf(`Package header and body: %+s`, structsFileString.String())
	// Write the prepared code with generated structures to file.
	sep := string(os.PathSeparator)
	path := strings.Split(dirName, sep)
	packageName := path[len(path)-1]
	structsFileName := dirName + sep + packageName + "_structs.go"
	if err = os.WriteFile(structsFileName, []byte(structsFileString.String()), 0600); err != nil {
		return fmt.Errorf("os.WriteFile: %w", err)
	}
	// Now we will know if we are ran for the first time for this directory or not.
	files, _ := dh.ReadDir(0)
	regenerated := false
	packageFileName := packageName + ".go"
	for _, f := range files {
		if f.Name() == packageFileName {
			regenerated = true
		}
	}
	if !regenerated {
		modelAsString := prepareModelFileContents(packageName)
		modelFileName := dirName + sep + packageFileName
		return os.WriteFile(modelFileName, []byte(modelAsString), 0600)
	}
	return err
}

var modelHeader = `// Package ${package} contains structs mapped to tables, produced from
// database ${database}.
// They all implement the [rx.SqlxMeta] interface and can be used
// for CRUD operations.
package ${package}

/*
This file will not be regenerated the next time you run [rx.Generate]. You can
add your custom code here.
*/
`

func prepareModelFileContents(packageName string) string {
	return replace(modelHeader, `${`, `}`, map[string]any{
		`package`:  packageName,
		`Package`:  SnakeToCamel(packageName),
		`database`: DSN,
	})
}

var packageHeader = `package ${package}
/*
This file will be regenerated each time you run [rx.Generate]
*/

import (
	"database/sql"
	"time"
	
	"github.com/kberov/rowx/rx"
)

`

// preparePackageHeaderForGeneratedStructs only iterates trough the rows to prepare the Rowx
// constraint. It allso uses the last folder from packagePath for package name.
// The produced string is added to fileString.
func preparePackageHeaderForGeneratedStructs(packagePath string, fileString *strings.Builder) {
	pathToPackage := strings.Split(packagePath, string(os.PathSeparator))
	packageName := pathToPackage[len(pathToPackage)-1]
	fileString.WriteString(
		replace(packageHeader, `${`, `}`, SQLMap{
			`package`:  packageName,
			`Package`:  SnakeToCamel(packageName),
			`database`: DSN,
		}),
	)
}

var structTemplate = `

// New${TableName} is a constructor for [${TableName}].
var New${TableName} = rx.NewRx[${TableName}]

var _ rx.SqlxModel[${TableName}] = New${TableName}()

// ${TableName} is an object, mapped to ${table_name}. It implements the
// SqlxMeta interface. 
type ${TableName} struct {
${fields}
}

// Table returns ${table_name} for ${TableName}.
func (u *${TableName}) Table() string {
	return "${table_name}" 
}

// Columns returns a slice with column_names for ${TableName}.
func (u *${TableName}) Columns() []string {
	return []string{${column_names}
	}
}
`

func appendRowToLastStructTemplate(structsStashes *[]map[string]any, i int, columns []columnInfo) {
	c := 0
	columnName := "\n\t\t\"" + columns[i].CName + "\","
	if i == 0 {
		// SA4006: this value of structsStashes is never used (staticcheck)
		//nolint:staticcheck
		*structsStashes = append(*structsStashes, map[string]any{
			`TableName`:    SnakeToCamel(columns[i].TableName),
			`table_name`:   columns[i].TableName,
			`fields`:       sql2GoTypeAndTag(columns[i]),
			`column_names`: columnName,
		})
		return
	}

	l := len(*structsStashes)
	c = l - 1

	if (*structsStashes)[c][`table_name`] != columns[i].TableName {
		// SA4006: this value of structsStashes is never used (staticcheck)
		//nolint:staticcheck
		*structsStashes = append(*structsStashes, map[string]any{
			`TableName`:    SnakeToCamel(columns[i].TableName),
			`table_name`:   columns[i].TableName,
			`fields`:       sql2GoTypeAndTag(columns[i]),
			`column_names`: columnName,
		})
		return
	}
	(*structsStashes)[c][`fields`] = (*structsStashes)[c][`fields`].(string) + sql2GoTypeAndTag(columns[i])
	(*structsStashes)[c][`column_names`] = (*structsStashes)[c][`column_names`].(string) + columnName
}

// sql2GoTypeAndTag converts SQL column types to Go types. Parts of the code
// were shamelessly stollen from https://github.com/go-jet/jet
// generator/template/model_template.go: toGoType(column metadata.Column).
func sql2GoTypeAndTag(column columnInfo) string {
	// Logger.Debugf(`column.CType:%s;column.NotNull:%v`, column.CType, column.NotNull)
	var colType = strings.ToLower(strings.TrimSpace(strings.Split(column.CType, "(")[0]))
	var goType string

	switch colType {
	case "user-defined", "enum":
		goType = sql2IfNullableGoType(column, "string")
	case "boolean", "bool":
		goType = sql2IfNullableGoType(column, "bool")
	case "tinyint":
		goType = sql2IfNullableGoType(column, "int8")
	case "smallint", "int2", "year":
		goType = sql2IfNullableGoType(column, "int16")
	case "integer", "int4",
		"mediumint", "int": // MySQL
		goType = sql2IfNullableGoType(column, "int32")
	case "bigint", "int8":
		goType = sql2IfNullableGoType(column, "int64")
	case "date",
		"timestamp without time zone", "timestamp",
		"timestamp with time zone", "timestamptz",
		"time without time zone", "time",
		"time with time zone", "timetz",
		"datetime": // MySQL
		goType = sql2IfNullableGoType(column, "time.Time")
	case "bytea",
		"binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob": // MySQL
		goType = sql2IfNullableGoType(column, "[]byte")
	case "text",
		"character", "bpchar",
		"character varying", "varchar", "nvarchar",
		"tsvector", "bit", "bit varying", "varbit",
		"money", "json", "jsonb",
		"xml", "point", "interval", "line", "array",
		"char", "tinytext", "mediumtext", "longtext": // MySQL
		goType = sql2IfNullableGoType(column, "string")
	case "real", "float4":
		goType = sql2IfNullableGoType(column, "float32")
	case "numeric", "decimal",
		"double precision", "float8", "float",
		"double": // MySQL
		goType = sql2IfNullableGoType(column, "float64")
	default:
		Logger.Infof("Unsupported sql column type '%s' for column '%s', using string instead.", column.CType, column.CName)
		goType = sql2IfNullableGoType(column, "string")
	}
	// Logger.Debugf("goType:%s", goType)
	var neededTag string
	columnName := strings.ToLower(column.CName)
	if columnName == `id` {
		neededTag = "`" + ReflectXTag + `:"` + columnName + `,auto"` + "`"
	}
	field := "\t" + SnakeToCamel(columnName) + ` ` + goType + " " + neededTag + "\n"

	return field
}

/*
sql2IfNullableGoType decides what will be the final type for the field in the
Go struct. We may add here some heuristics applied on the data and found check
constraints to set our own types which implement the Valuer and Scanner
interfaces.
*/
func sql2IfNullableGoType(column columnInfo, defaultType string) string {
	if column.PK > 0 {
		return defaultType
	}
	if column.NotNull {
		return defaultType
	}
	return "sql.Null[" + defaultType + "]"
}

func prepareGeneratedStructs(columns []columnInfo, fileString *strings.Builder) {
	structsInfo := make([]map[string]any, 0, 10)

	for i := range columns {
		appendRowToLastStructTemplate(&structsInfo, i, columns)
	}
	// Logger.Debugf(`structsInfo: %+v`, structsInfo)
	for _, v := range structsInfo {
		fileString.WriteString(replace(structTemplate, `${`, `}`, v))
	}
}

type columnInfo struct {
	TableName string
	CID       uint8
	CName     string
	// CType sql.ColumnType
	CType        string
	NotNull      bool
	DefaultValue sql.NullString
	PK           uint8
	SQL          string `rx:"sql"`
}

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
	"sort"
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
	// Logger.Debugf("TypeToSnakeCase typestr: %s", typestr)
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
		if WORD, ok := isCommonInitialism(word); ok {
			splitWords[i] = WORD
			continue
		}
		splitWords[i] = strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:]))
	}
	return strings.Join(splitWords, ``)
}

// isCommonInitialism checks and returns the uppercased or properly modified word and `true`. if a word is not an initialism it returns it unchanged and returns `false`.
func isCommonInitialism(word string) (string, bool) {
	switch word {
	case `acl`, `api`, `ascii`, `cpu`, `css`, `dns`, `eof`, `eta`, `gpu`,
		`guid`, `html`, `http`, `https`, `id`, `ip`, `json`, `lhs`, `os`, `qps`,
		`ram`, `rhs`, `rpc`, `sla`, `smtp`, `sql`, `ssh`, `tcp`, `tls`, `ttl`,
		`udp`, `ui`, `uid`, `uuid`, `uri`, `url`, `utf8`, `vm`, `xml`, `xmpp`,
		`xsrf`, `xss`, `pid`:
		return strings.ToUpper(word), true
	case `oauth`:
		return `OAuth`, true
	case `OAuth`:
		return word, true
	default:
		return word, false
	}
}

type dir uint8

const (
	up dir = iota
	down
)

var updown = [...]string{`up`, `down`}

func (d dir) String() string {
	return updown[d]
}

/*
Migrate executes all not applied schema migrations with the given `direction`,
found in `filePath` and stores in [MigrationsTable] the version, direction and
file path of every applied migration. The migrations comments (headers) are
expected to mach `^--\s*(\d{1,12})\s*(up|down)$`. For example: `--202506092333
up`. All SQL statements in a migration are executed at once as one
transaction.

If the `direction` is `up`, all migrations in a file are applied in FIFO order.

If the `direction` is `down`, all migrations in a file are applied in LIFO order.

The explained workflow allows to have more than one migration (a set of
statements) in the same file for logically different parts of the application.
For example different modules have their own different migrations but they in
some cases have to be applied in one run - a new release.

Migrate is often followed by executing [Generate], if the schema of the
database is modified - new columns or tables are added, modified or removed
etc.
*/
func Migrate(filePath, dsn, direction string) error {
	if unknown(direction) {
		return fmt.Errorf(`direction can be only '%s' or '%s'`, up, down)
	}
	/*
		FIXME: dangerous!!! we assume here that DB() was not invoked yet and
		Migrate is called from a main() function. What if it is called from a
		long-running process? We need another separate singleDB.
	*/
	DSN = dsn
	DB().MustExec(RenderSQLTemplate(`CREATE_MIGRATIONS_TABLE`, Map{`table`: MigrationsTable}))

	migrations, err := parseMigrationFile(filePath)
	if err != nil {
		return err
	}
	if direction == down.String() {
		slices.Reverse(migrations)
	}

	for _, v := range migrations {
		statements := v.Statements.String()
		if v.Direction != direction {
			Logger.Infof(`Unaplicable %s %s: %s...`, v.Version, v.Direction, substr(statements, 30))
			continue
		}
		Logger.Infof(`Applying %s %s: %s...`, v.Version, v.Direction, substr(statements, 30))

		if err = multiExec(DB(), statements); err != nil {
			return err
		}
		if _, err = NewRx(Migrations{
			Version:   v.Version,
			Direction: v.Direction,
			FilePath:  filePath}).Insert(); err != nil {
			return err
		}
	}
	return err
}

func substr(str string, lenChars int) string {
	var newStr strings.Builder
	for i, char := range str {
		newStr.WriteRune(char)
		if i == lenChars-1 {
			break
		}
	}
	return newStr.String()
}

func unknown(direction string) bool {
	return direction != up.String() && direction != down.String()
}

/*
multiExec was stollen from sqlx_test.go and slightly modified as a poor's man
migration. It executes all stements, found in a big multy query string, at once
and in one transaction.
*/
func multiExec(db *sqlx.DB, query string) (err error) {
	tx := db.MustBegin()
	// The rollback will be ignored if the tx has been committed already.
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(query)
	if err != nil {
		return err
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
			v, err := NewRx[Migrations]().Get(
				`version=:ver AND direction =:dir`, Map{`ver`: version, `dir`: direction})
			// If this migration is not found in the applied migrations, we
			// must start collecting its lines to apply it.
			if err != nil && errors.Is(err, sql.ErrNoRows) {
				versionIsApplied = false
				currentVersion = version
				migrations = append(migrations,
					migration{Version: currentVersion, Direction: direction})
			} else if err == nil {
				Logger.Infof(`applied "%s %s" during a previous run...`, v.Version, v.Direction)
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
Generate generates structures for tables, found in database, pointed to by
`dsn` and dumps them to a given `packagePath` directory. Returns an error if
unsuccessful at any point of the execution. The name of the last directory in
the path is used as package name. The directory must exist already.

Two files are created. The first only declares the package and can be modified
by the programmer. It will not be regenerated on subsequent runs. The second
contains all the structures, mapped to tables. It will be regenerated again on
the next run of this function to map the potentially migrated to a new state
schema to Go structs.
*/
func Generate(dsn string, packagePath string) error {
	DSN = dsn
	dh, err := safeOpen(packagePath)
	if err != nil {
		return fmt.Errorf("%w. The directory must exist already", err)
	}
	defer dh.Close()
	sql := QueryTemplates[`SELECT_TABLE_INFO_sqlite3`].(string)
	info := []columnInfo{}
	if err = DB().Select(&info, sql, MigrationsTable); err != nil {
		return err
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
	// TODO: Generate also a file for views.
	tablesFileName := dirName + sep + packageName + "_tables.go"
	// Now we will know if we are ran for the first time for this directory or not.
	files, _ := dh.ReadDir(0)
	regenerated := false
	packageFileName := packageName + ".go"
	rePrefix := ``
	for _, f := range files {
		if f.Name() == packageFileName {
			regenerated = true
			rePrefix = `re-`
		}
	}
	Logger.Infof(`%sgenerating %s...`, rePrefix, tablesFileName)
	if err = os.WriteFile(tablesFileName, []byte(structsFileString.String()), 0600); err != nil {
		return fmt.Errorf("os.WriteFile: %w", err)
	}
	if !regenerated {
		modelAsString := prepareModelFileContents(packageName)
		modelFileName := dirName + sep + packageFileName
		Logger.Infof(`generating %s...`, modelFileName)
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
// TODO: Import only used packages. Until then we use goimports to clean unused packages.
func preparePackageHeaderForGeneratedStructs(packagePath string, fileString *strings.Builder) {
	pathToPackage := strings.Split(packagePath, string(os.PathSeparator))
	packageName := pathToPackage[len(pathToPackage)-1]
	fileString.WriteString(
		replace(packageHeader, `${`, `}`, Map{
			`package`:  packageName,
			`Package`:  SnakeToCamel(packageName),
			`database`: DSN,
		}),
	)
}

var structTemplate = `

// New${TableName} is a constructor for rx.SqlxModel[${TableName}].
func New${TableName}(rows...${TableName}) rx.SqlxModel[${TableName}] {
	return rx.NewRx[${TableName}](rows...)
}

var _ rx.SqlxModel[${TableName}] = New${TableName}()

// ${TableName} is an object, mapped to table ${table_name}. It implements the
// SqlxMeta interface. 
type ${TableName} struct {
${fields}
}

// Table returns the table name ${table_name} for ${TableName}.
func (u *${TableName}) Table() string {
	return "${table_name}" 
}

// Columns returns a slice, containing column names for ${TableName}.
func (u *${TableName}) Columns() []string {
	return []string{${column_names}
	}
}
`

func appendRowToLastStructTemplate(structsStashes *[]Map, i int, columns []columnInfo) {
	last := 0
	columnName := "\n\t\t\"" + columns[i].CName + `",`
	if i == 0 {
		fieldsWithGoTypes := make([]fieldWithGoType, 0, 10)
		// SA4006: this value of structsStashes is never used (staticcheck)
		//nolint:staticcheck
		*structsStashes = append(*structsStashes, Map{
			`TableName`:         SnakeToCamel(columns[i].TableName),
			`table_name`:        columns[i].TableName,
			`fieldsWithGoTypes`: &fieldsWithGoTypes,
			`fields`:            sql2GoTypeAndTag(columns[i], &fieldsWithGoTypes),
			`column_names`:      columnName,
		})
		return
	}

	l := len(*structsStashes)
	last = l - 1
	// Start a new stash for a struct(table) if the next column's TableName is
	// different.
	if (*structsStashes)[last][`table_name`] != columns[i].TableName {
		fieldsWithGoTypes := make([]fieldWithGoType, 0, 10)
		// SA4006: this value of structsStashes is never used (staticcheck)
		//nolint:staticcheck
		*structsStashes = append(*structsStashes, Map{
			`TableName`:         SnakeToCamel(columns[i].TableName),
			`table_name`:        columns[i].TableName,
			`fieldsWithGoTypes`: &fieldsWithGoTypes,
			`fields`:            sql2GoTypeAndTag(columns[i], &fieldsWithGoTypes),
			`column_names`:      columnName,
		})
		return
	}
	// Always work with the lastly appended struct data.
	fieldsWithGoTypes := (*structsStashes)[last][`fieldsWithGoTypes`].(*[]fieldWithGoType)
	(*structsStashes)[last][`fields`] = (*structsStashes)[last][`fields`].(string) + sql2GoTypeAndTag(columns[i], fieldsWithGoTypes)
	(*structsStashes)[last][`column_names`] = (*structsStashes)[last][`column_names`].(string) + columnName
}

type fieldWithGoType struct {
	field, goType string
}

// sql2GoTypeAndTag converts SQL column types to Go types. Case statemnets
// were shamelessly stollen from https://github.com/go-jet/jet
// generator/template/model_template.go: toGoType(column metadata.Column).
func sql2GoTypeAndTag(column columnInfo, fieldsSlice *[]fieldWithGoType) string {
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
	case "int4",
		"mediumint", "int": // MySQL
		goType = sql2IfNullableGoType(column, "int32")
	case "integer", "bigint", "int8":
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
		neededTag = " `" + ReflectXTag + `:"` + columnName + `,auto"` + "`"
	}
	field := "\t" + SnakeToCamel(columnName) + ` ` + goType + neededTag + "\n"
	*fieldsSlice = append(*fieldsSlice, fieldWithGoType{field, goType})
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
	structsInfo := make([]Map, 0, 10)

	for i := range columns {
		appendRowToLastStructTemplate(&structsInfo, i, columns)
	}
	// Logger.Debugf(`structsInfo: %+v`, structsInfo)
	for _, v := range structsInfo {
		allignStructFields(v)
		fileString.WriteString(replace(structTemplate, `${`, `}`, v))
	}
}

type columnInfo struct {
	SQL       string `rx:"sql"`
	TableName string
	CName     string
	// CType sql.ColumnType
	CType        string
	DefaultValue sql.NullString
	CID          uint8
	PK           uint8
	NotNull      bool
}

func allignStructFields(structInfo Map) {
	columns := *(structInfo[`fieldsWithGoTypes`].(*[]fieldWithGoType))
	sort.Slice(columns, func(i, j int) bool {
		ai := alignTable[columns[i].goType]
		aj := alignTable[columns[j].goType]
		if ai == aj {
			return sizeTable[columns[i].goType] > sizeTable[columns[j].goType]
		}
		return ai > aj
	})
	// Logger.Debugf(`aligned fieldsWithGoTypes: [%+v]`, structInfo[`fieldsWithGoTypes`])
	var alignedFields strings.Builder
	for _, v := range columns {
		alignedFields.WriteString(v.field)
	}
	structInfo[`fields`] = alignedFields.String()
}

var alignTable = map[string]int{
	// Основни типове
	"bool":    1,
	"int8":    1,
	"uint8":   1,
	"int16":   2,
	"uint16":  2,
	"int32":   4,
	"uint32":  4,
	"float32": 4,
	"int64":   8,
	"uint64":  8,
	"float64": 8,
	"int":     8, // 64-бит
	"string":  8,
	"[]byte":  8,

	// Често срещани типове
	"time.Time": 8,

	// Класически Null типове
	"sql.NullInt64":   8,
	"sql.NullFloat64": 8,
	"sql.NullBool":    8,
	"sql.NullString":  8,

	// Обобщен Null[T] - Go 1.22+
	"sql.Null[int8]":      1,
	"sql.Null[uint8]":     1,
	"sql.Null[int16]":     2,
	"sql.Null[uint16]":    2,
	"sql.Null[int32]":     4,
	"sql.Null[uint32]":    4,
	"sql.Null[float32]":   4,
	"sql.Null[int64]":     8,
	"sql.Null[uint64]":    8,
	"sql.Null[float64]":   8,
	"sql.Null[string]":    8, // string е pointer+len, align=8
	"sql.Null[time.Time]": 8,
	"sql.Null[[]byte]":    8,
}

var sizeTable = map[string]int{
	"bool":    1,
	"int8":    1,
	"uint8":   1,
	"int16":   2,
	"uint16":  2,
	"int32":   4,
	"uint32":  4,
	"float32": 4,
	"int64":   8,
	"uint64":  8,
	"float64": 8,
	"int":     8,
	"string":  16,
	"[]byte":  24,

	// Често срещани типове
	"time.Time": 24,

	// Класически Null типове
	"sql.NullInt64":   16,
	"sql.NullFloat64": 16,
	"sql.NullBool":    16,
	"sql.NullString":  32,

	// Обобщени Null[T] Го 1.22+ (приблизителни стойности)
	"sql.Null[int8]":      2,
	"sql.Null[uint8]":     2,
	"sql.Null[int16]":     4,
	"sql.Null[uint16]":    4,
	"sql.Null[int32]":     8,
	"sql.Null[uint32]":    8,
	"sql.Null[float32]":   8,
	"sql.Null[int64]":     16,
	"sql.Null[uint64]":    16,
	"sql.Null[float64]":   16,
	"sql.Null[string]":    32,
	"sql.Null[time.Time]": 32,
	"sql.Null[[]byte]":    40,
}

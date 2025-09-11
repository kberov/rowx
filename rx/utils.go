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
func SnakeToCamel(snake_case_word string) string { //nolint:all //  should be snakeCaseWord
	runes := []rune(snake_case_word)
	if len(runes) == 2 {
		return strings.ToUpper(snake_case_word)
	}
	var words strings.Builder
	nextUp := false

	words.WriteRune(unicode.ToUpper(runes[0]))
	for _, v := range runes[1:] {
		if v == '_' {
			nextUp = true
			continue
		}
		if nextUp {
			words.WriteRune(unicode.ToUpper(v))
			nextUp = false
			continue
		}
		words.WriteRune(v)
	}
	return words.String()
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
	Logger.Debugf(`Opening migrations file %s`, filePath)
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
Generate generates structures for tables, found in database pointed to by `dsn`
and dumps them to a given `packagePath` directory. Returna an error if
unsuccesfull. The the name of the last directory in the path is used as
package name. The directory must exist already.

Three files are created. The first consist of boilerplate code. There is a
generic structure embedding [Rx] constrained to only the generated from tables
structures and can be modified by the programmer. The second contains all the
structures, mapped to tables. The third is a test file, containing tests for
the generated structures to prove that the generated code works fine. All three
files should be under version control.
*/
func Generate(dsn string, packagePath string) error {
	DSN = dsn
	dh, err := safeOpen(packagePath)
	if err != nil {
		return err
	}
	defer dh.Close()
	// Now we will know if we are run for the first time or not.
	files, err := dh.ReadDir(0)
	if err != nil {
		return err
	}
	// `reGenerate` will have the value 0 if we open the directory for the first time.
	reGenerate := len(files)
	_ = reGenerate
	return err
}

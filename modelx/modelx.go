// Package modelx provides an interface and an abstract generic data type
// implementing it for use with github.com/jmoiron/sqlx.
package modelx

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/gommon/log"
	"github.com/valyala/fasttemplate"
)

var (
	// DefaultLogHeader is a template for modelx logging
	DefaultLogHeader = `${prefix}:${time_rfc3339}:${level}:${short_file}:${line}`
	// Default LIMIT for SQL queries.
	DefaultLimit = 100
	// DSN must be set before using DB() function.
	DSN string
	// Logger must be instantiated before using any function from this package.
	Logger *log.Logger
	// singleDB is a singleton connection to the database.
	singleDB *sqlx.DB
	sprintf  = fmt.Sprintf
)

func DB() *sqlx.DB {
	if singleDB != nil {
		return singleDB
	}
	if Logger == nil {
		Logger = log.New("DB")
		Logger.SetOutput(os.Stderr)
		Logger.SetHeader(DefaultLogHeader)
		Logger.SetLevel(log.DEBUG)
	}
	Logger.Debugf("Connecting to database '%s'...", DSN)

	singleDB = sqlx.MustConnect("sqlite3", DSN)
	singleDB.MapperFunc(camelToSnakeCase)
	return singleDB
}

/*
SqlxRow is an interface and generic constraint for rows. TODO? See if we need
to  implement this interface or the Modelx will work well.
*/
type SqlxRow interface {
	// Insert this prepared record into it's table.
	// Insert() error
	// Select (Get) one record by ID
	// Get() error
	// Update this record.
	// Update() error
	// Delete this record
	// Delete() error
}

/*
SqlxModel is an interface and generic constraint.
*/
type SqlxModel[R SqlxRow] interface {
	Table() string
	Columns() []string
	Data() []R
	Insert() (sql.Result, error)
	Select(string, any, [2]int) error
	SqlxRow
}

type Modelx[R SqlxRow] struct {
	// table allows to set explicitly the table name for this model. Otherwise
	// it is guessed and set from the type of the Data slice upon first use of
	// TableName().
	table   string
	columns []string
	data    []R
}

// NewModel returns a new instance of a table model with optional slice of
// provided data rows as a variadic parameter.
func NewModel[R SqlxRow](rows ...R) SqlxModel[R] {
	if rows != nil {
		return &Modelx[R]{data: rows}
	}
	return &Modelx[R]{}
}

// Table returns the guessed table name from the parametrized Data type.
func (m *Modelx[R]) Table() string {
	if m.table != "" {
		return m.table
	}
	m.table = modelToTable(new(R))
	return m.table
}

// modelToTable converts struct type name like *model.Users to
// 'users' and returns it. Panics if unsuccessful.
func modelToTable[R SqlxRow](rows R) string {
	typestr := sprintf("%T", rows)
	_, table, ok := strings.Cut(typestr, ".")
	if ok {
		return camelToSnakeCase(table)
	}
	panic(sprintf("Could not derive table name from type '%s'!", typestr))
}

// camelToSnakeCase is used to convert structure fields to
// snake case table columns by sqlx.DB.MapperFunc. See tests for examples.
func camelToSnakeCase(text string) string {
	if utf8.RuneCountInString(text) == 2 {
		return strings.ToLower(text)
	}
	var snakeCase strings.Builder
	var wordBegins = true
	var prevWasUpper = true
	for _, r := range text {
		wordBegins, prevWasUpper = lowerLetter(&snakeCase, r, wordBegins, prevWasUpper, "_")
	}
	return snakeCase.String()
}

func lowerLetter(snakeCase *strings.Builder, r rune, wordBegins, prevWasUpper bool, connector string) (bool, bool) {
	if unicode.IsUpper(r) && !wordBegins {
		snakeCase.WriteString(connector)
		snakeCase.WriteRune(unicode.ToLower(r))
		wordBegins = true
		prevWasUpper = true
		return wordBegins, prevWasUpper
	}
	// handle case `ID` and beginning of word
	if wordBegins && prevWasUpper {
		snakeCase.WriteRune(unicode.ToLower(r))
		wordBegins = false
		prevWasUpper = false
		return wordBegins, prevWasUpper
	}
	snakeCase.WriteRune(r)
	return wordBegins, prevWasUpper
}

// Data returns the slice of structs, passed to NewModel().
func (m *Modelx[R]) Data() []R {
	return m.data
}

// Columns returns a slice with the names of the columns of the table in no
// particular order. TODO! See if sqlx copes with the given order in an insert
// statement.
func (m *Modelx[R]) Columns() []string {
	if m.columns != nil {
		return m.columns
	}
	colMap := DB().Mapper.FieldMap(reflect.ValueOf(new(R)))
	m.columns = make([]string, 0, len(colMap)/2)
	for k := range colMap {
		if strings.Contains(k, `.`) {
			continue
		}
		m.columns = append(m.columns, k)
	}
	return m.columns
}

// Insert inserts a set of SqlxRow instances and returns sql.Result and error.
// If len(m.Data())>1 the data is inserted in a transaction. If
// len(m.Data())=0, it panics. If [QueryTemplates][`INSERT`] is not found, it
// panics.
func (m *Modelx[R]) Insert() (sql.Result, error) {
	dataLen := len(m.Data())
	// Logger.Debugf("Data: %#v", m.data)
	if dataLen == 0 {
		panic("Cannot insert when no data is provided!")
	}
	template := getQueryTemplate(`INSERT`)
	placeholders := strings.Join(m.Columns(), ",:") // :id,:login_name,:changed_by...
	placeholders = sprintf("(:%s)", placeholders)
	stash := map[string]any{
		`columns`:      strings.Join(m.Columns(), ","),
		`table`:        m.Table(),
		`placeholders`: placeholders,
	}
	query := fasttemplate.ExecuteStringStd(template, "${", "}", stash)
	// Logger.Debugf("INSERT query from fasttemplate: %s", query)
	if dataLen > 1 {
		tx := DB().MustBegin()
		var (
			r sql.Result
			e error
		)
		for _, row := range m.Data() {
			r, e := tx.NamedExec(query, row)
			if e != nil {
				return r, e
			}
		}
		if e := tx.Commit(); e != nil {
			return nil, e
		}
		return r, e

	}
	return DB().NamedExec(query, m.Data()[0])
}

func getQueryTemplate(key string) string {
	var (
		template string
		ok       bool
	)
	if template, ok = QueryTemplates[key].(string); !ok {
		panic("Query template for `INSERT` was not found in modelx.QueryTemplates!")
	}
	return template
}

// Select prepares and executes a [sqlx.NamedQuery]
func (m *Modelx[R]) Select(where string, bindData any, limitAndOffcet [2]int) error {
	template := getQueryTemplate(`SELECT`)
	if limitAndOffcet[0] == 0 {
		limitAndOffcet[0] = DefaultLimit
	}
	if bindData == nil {
		bindData = map[string]any{}
	}
	stash := map[string]any{
		`columns`: strings.Join(m.Columns(), ","),
		`table`:   m.Table(),
		`WHERE`:   where,
		`limit`:   strconv.Itoa(limitAndOffcet[0]),
		`offset`:  strconv.Itoa(limitAndOffcet[1]),
	}
	query := fasttemplate.ExecuteStringStd(template, "${", "}", stash)
	Logger.Debugf("Constructed query : %s", query)
	if stmt, err := DB().PrepareNamed(query); err != nil {
		return fmt.Errorf("error from DB().PrepareNamed(SQL): %w", err)
	} else if err = stmt.Select(&m.data, bindData); err != nil {
		return fmt.Errorf("error from stmt.Select(...): %w", err)
	}
	return nil
}

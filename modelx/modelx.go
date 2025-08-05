// Package modelx provides an interface and an abstract generic data type
// implementing it for use with github.com/jmoiron/sqlx.
package modelx

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/gommon/log"
)

var (
	// DefaultLogHeader is a template for modelx logging
	DefaultLogHeader = `${prefix}:${time_rfc3339}:${level}:${short_file}:${line}`

	// DSN must be set before using DB() function.
	DSN string
	// Logger must be instantiated before using any function from this package.
	Logger *log.Logger
	// global is a singleton connection to the database.
	global  *sqlx.DB
	sprintf = fmt.Sprintf
)

func DB() *sqlx.DB {
	if global != nil {
		return global
	}
	if Logger == nil {
		Logger = log.New("DB")
		Logger.SetOutput(os.Stderr)
		Logger.SetHeader(DefaultLogHeader)
		Logger.SetLevel(log.DEBUG)
	}
	Logger.Debugf("Connecting to database '%s'...", DSN)

	global = sqlx.MustConnect("sqlite3", DSN)
	global.MapperFunc(camelToSnakeCase)
	return global
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
	TableName() string
	Columns() []string
	Data() []R
	SqlxRow
}

type Modelx[R SqlxRow] struct {
	// Table allows to set explicitly the table name for this model. Otherwise
	// it is guessed and set from the type of the Data slice upon first use of
	// TableName().
	Table   string
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

// TableName returns the guessed table name from the paramaetrized Data type.
func (m *Modelx[R]) TableName() string {
	if m.Table != "" {
		return m.Table
	}
	m.Table = modelToTable(m.Data)
	return m.Table
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

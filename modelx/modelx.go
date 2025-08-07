/*
Package modelx provides two interfaces and an abstract generic data types
implementing them to work easily with database records and sets of records.
Underneath github.com/jmoiron/sqlx is used. It is just an object mapper. The
relations' constraints are left to be managed by the database
*/
package modelx

import (
	"database/sql"
	"fmt"
	"maps"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/gommon/log"
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
SqlxRow is an interface and generic constraint for one record. TODO? See if we
need to implement this interface or the Modelx will be enough.
*/
type SqlxRow interface {
	// Insert this prepared record into it's table.
	// Insert() error - TODO: insert record with specific ID value
	// Select (Get) one record by ID
	// GetByID() error - TODO
	// Update this record.
	// Update() error
	// Delete this record
	// Delete() error
}

/*
SqlxModel is an interface and generic constraint for a set of records.
*/
type SqlxModel[R SqlxRow] interface {
	SqlxRow
	Table() string
	Columns() []string
	Data() []R
	Insert() (sql.Result, error)
	Select(string, any, [2]int) error
	Update(map[string]any, string, map[string]any) (sql.Result, error)
}

/*
Modelx implements SqlxModel interface and can be embedded (extended) to
customise its behaviour for your own needs.
*/
type Modelx[R SqlxRow] struct {
	// data is a slice of rows, retrieved from the database or to be inserted,
	// updated or deleted.
	data []R
	// table allows to set explicitly the table name for this model. Otherwise
	// it is guessed and set from the type of the first element of Data slice
	// upon first use of Table().
	table string
	// columns of the table
	columns []string
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
// snake case table columns by sqlx.DB.MapperFunc.
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

// Data returns the slice of structs, passed to NewModel(). It may return nil
// if no rows are passed.
func (m *Modelx[R]) Data() []R {
	return m.data
}

/*
Columns returns a slice with the names of the columns of the table in no
particular order. Because the order is different each time, we must use
internally NamedQuery, NamedExec, PrepareNamed etc. from sqlx.
*/
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

/*
Insert inserts a set of SqlxRow instances (without their ID values) and returns
sql.Result and error. The value for the ID column is left to be set by the
database. If len(m.Data())>1 the data is inserted in a transaction. If
len(m.Data())=0, it panics. If [QueryTemplates][`INSERT`] is not found, it
panics.
If you need to insert an SqlxRow structure with a specific value for ID, use
[SqlxRow.Insert](TODO).
*/
func (m *Modelx[R]) Insert() (sql.Result, error) {
	dataLen := len(m.Data())
	// Logger.Debugf("Data: %#v", m.data)
	if dataLen == 0 {
		Logger.Panic("Cannot insert, when no data is provided!")
	}
	colsNoID := m.colsWithoutID()
	placeholders := strings.Join(colsNoID, ",:") // :login_name,:changed_by...
	placeholders = sprintf("(:%s)", placeholders)
	stash := map[string]any{
		`columns`:      strings.Join(colsNoID, ","),
		`table`:        m.Table(),
		`placeholders`: placeholders,
	}
	query := RenderSQLFor(`INSERT`, stash)
	Logger.Debugf("INSERT query from fasttemplate: %s", query)
	if dataLen > 1 {
		var (
			tx *sqlx.Tx
			r  sql.Result
			e  error
		)
		if tx, e = DB().Beginx(); e != nil {
			return nil, e
		}
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

// colsWithoutID retrurns a new slice, which does not contain the 'id' element.
func (m *Modelx[R]) colsWithoutID() []string {
	cols := m.Columns()
	placeholdersForInsert := make([]string, 0, len(cols)-1)
	for _, v := range cols {
		if v == "id" {
			continue
		}
		placeholdersForInsert = append(placeholdersForInsert, v)
	}
	return placeholdersForInsert
}

/*
Select prepares and executes a [sqlx.NamedQuery]. Selected records can be used
with [SqlxModel.Data].
*/
func (m *Modelx[R]) Select(where string, bindData any, limitAndOffset [2]int) error {
	if limitAndOffset[0] == 0 {
		limitAndOffset[0] = DefaultLimit
	}
	if bindData == nil {
		bindData = map[string]any{}
	}
	stash := map[string]any{
		`columns`: strings.Join(m.Columns(), ","),
		`table`:   m.Table(),
		`WHERE`:   where,
		`limit`:   strconv.Itoa(limitAndOffset[0]),
		`offset`:  strconv.Itoa(limitAndOffset[1]),
	}
	query := RenderSQLFor(`SELECT`, stash)
	Logger.Debugf("Constructed query : %s", query)
	if stmt, err := DB().PrepareNamed(query); err != nil {
		return fmt.Errorf("error from DB().PrepareNamed(SQL): %w", err)
	} else if err = stmt.Select(&m.data, bindData); err != nil {
		return fmt.Errorf("error from stmt.Select(&m.data, bindData): %w", err)
	}
	return nil
}

/*
Update constructs a Named UPDATE query and executes it. setData contains data
to be set. bindData contains data for the WHERE clause. You have to make
different names for the same fields to be set and used in WHERE clause, because
these are merged together and passed to sqlx as one map. If there are keys with
the same name, entries from setData will overwrite those in bindData. This will
lead to wrongly updated data in the database.
*/
func (m *Modelx[R]) Update(setData map[string]any, where string, bindData map[string]any) (sql.Result, error) {
	stash := map[string]any{
		`table`: m.Table(),
		`SET`:   buildSET(setData),
		`WHERE`: where,
	}
	if bindData == nil {
		bindData = make(map[string]any)
	}
	query := RenderSQLFor(`UPDATE`, stash)
	Logger.Debugf("Constructed query : %s", query)
	maps.Copy(bindData, setData)
	return DB().NamedExec(query, bindData)
}

func buildSET(bindData map[string]any) string {
	var set strings.Builder
	set.WriteString(`SET`)
	for key := range bindData {
		set.WriteString(sprintf(` %s = :%[1]s,`, key))
	}
	// s[:len(s)-1]
	// return strings.TrimRight(set.String(), `,`)
	setStr := set.String()
	return setStr[:len(setStr)-1]
}

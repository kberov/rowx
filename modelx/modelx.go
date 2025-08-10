/*
Package modelx provides interfaces and a generic data type, implementing
the interfaces to work easily with database records and sets of records.
Underneath [sqlx] is used. Package modelx provides just an object
mapper. The relations' constraints are left to be managed by the database.
If you embed (extend) the data type [Modelx], you get automatically
the respective implementation and can overwrite methods to customise them for
your needs.

Caveat: The current implementation naively assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. For now just use [sqlx] for such tables.
*/
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
)

var (
	// DefaultLogHeader is a template for modelx logging
	DefaultLogHeader = `${prefix}:${time_rfc3339}:${level}:${short_file}:${line}`
	// DefaultLimit is the default LIMIT for SQL queries.
	DefaultLimit = 100
	// DriverName is the name of the database engine to use.
	DriverName = `sqlite3`
	// DSN must be set before using DB() function.
	DSN string
	// Logger is instantiated (if not instantiated already externally) during
	// first call of DB() and the log level is set to log.DEBUG.
	Logger *log.Logger
	// singleDB is a singleton connection to the database.
	singleDB *sqlx.DB
	sprintf  = fmt.Sprintf
)

/*
DB  instantiates the [log.Logger], invokes [sqlx.MustConnect] and sets the
[sqlx.MapperFunc].
*/
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

	singleDB = sqlx.MustConnect(DriverName, DSN)
	singleDB.MapperFunc(camelToSnakeCase)
	return singleDB
}

/*
SqlxRows is an interface and generic constraint for database records. TODO? See
if we need to implement this interface or the Modelx will be enough.
*/
type SqlxRows interface {
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
SqlxModel is an interface and generic constraint for working with a set of
database records. [Modelx] fully implements SqlxModel. You can embed (extend)
Modelx to get automatically its implementation and override some of its
methods.
*/
type SqlxModel[R SqlxRows] interface {
	Data() []R
	SqlxModelInserter[R]
	SqlxModelSelector[R]
	SqlxModelUpdater[R]
	SqlxModelDeleter[R]
}

/*
SqlxModelInserter can be implemented to insert records in a table. It is fully
implemented by [Modelx]. You can embed (extend) Modelx to get automatically
its implementation and override some of its methods.
*/
type SqlxModelInserter[R SqlxRows] interface {
	Table() string
	Columns() []string
	Insert() (sql.Result, error)
}

/*
SqlxModelUpdater can be implemented to update records in a table. It is fully
implemented by [Modelx]. You can embed (extend) Modelx to get automatically
its implementation and override some of its methods.
*/
type SqlxModelUpdater[R SqlxRows] interface {
	Table() string
	Update(string, bool) (sql.Result, error)
}

/*
SqlxModelSelector can be implemented to select records from a table or view. It
is fully implemented by [Modelx]. You can embed (extend) Modelx to get
automatically its implementation and override some of its methods.
*/
type SqlxModelSelector[R SqlxRows] interface {
	Table() string
	Columns() []string
	Select(string, any, ...int) ([]R, error)
}

/*
SqlxModelDeleter can be implemented to delete records from a table. It is
fully implemented by [Modelx]. You can embed (extend) Modelx to get
automatically it's implementation and override some of its methods.
*/
type SqlxModelDeleter[R SqlxRows] interface {
	Table() string
	Delete(string, any) (sql.Result, error)
}

/*
Modelx implements SqlxModel interface and can be embedded (extended) to
customise its behaviour for your own needs.
*/
type Modelx[R SqlxRows] struct {
	/*
		'.data' is a slice of rows, retrieved from the database or to be inserted,
		or updated.
	*/
	data []R
	/*
		'.table' allows to set explicitly the table name for this model. Otherwise
		it is guessed and set from the type of the first element of Data slice
		upon first use of '.Table()'.
	*/
	table string
	// '.columns' of the table are populated upon first use of '.Columns()'.
	columns []string
}

// NewModel returns a new instance of a table model with optionally provided
// data rows as a variadic parameter.
func NewModel[R SqlxRows](rows ...R) SqlxModel[R] {
	if rows != nil {
		return &Modelx[R]{data: rows}
	}
	return &Modelx[R]{}
}

// Table returns the guessed table name from the Data type parameter.
func (m *Modelx[R]) Table() string {
	if m.table != "" {
		return m.table
	}
	m.table = modelToTable(new(R))
	return m.table
}

// modelToTable converts struct type name like *model.Users to
// 'users' and returns it. Panics if unsuccessful.
func modelToTable[R SqlxRows](rows R) string {
	typestr := sprintf("%T", rows)
	_, table, ok := strings.Cut(typestr, ".")
	if !ok {
		Logger.Panicf("Could not derive table name from type '%s'!", typestr)
	}
	return camelToSnakeCase(table)
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
Columns returns a slice with the names of the table's columns in no particular
order. Because the order may be different on each instantation of [Modelx], we
use internally [sqlx.NamedExec], [sqlx.DB.PrepareNamed] etc.
*/
func (m *Modelx[R]) Columns() []string {
	if m.columns != nil {
		return m.columns
	}
	colMap := DB().Mapper.TypeMap(reflect.ValueOf(new(R)).Type()).Names
	m.columns = make([]string, 0, len(colMap))
	for k := range colMap {
		if strings.Contains(k, `.`) {
			continue
		}
		m.columns = append(m.columns, k)
	}
	Logger.Debugf(`m.columns: %#v`, m.columns)
	return m.columns
}

/*
Insert inserts a set of SqlxRows instances (without their ID values) and returns
[sql.Result] and [error]. The value for the ID column is left to be set by the
database. If the records to be inserted are more than one, the data is inserted
in a transaction. If there are no records to be inserted, it panics. If
[QueryTemplates][`INSERT`] is not found, it panics.

If you need to insert an SqlxRows structure with a specific value for ID, use
directly some of the [sqlx] functionnalities.
*/
func (m *Modelx[R]) Insert() (sql.Result, error) {
	dataLen := len(m.data)
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
	query := RenderSQLTemplate(`INSERT`, stash)
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
		// The rollback will be ignored if the tx has been committed already.
		defer func() { _ = tx.Rollback() }()
		for _, row := range m.data {
			r, e = tx.NamedExec(query, row)
			if e != nil {
				return r, e
			}
		}
		if e := tx.Commit(); e != nil {
			return r, e
		}
		return r, e

	}
	return DB().NamedExec(query, m.data[0])
}

// colsWithoutID retrurns a new slice, which does not contain the 'id' element.
func (m *Modelx[R]) colsWithoutID() []string {
	cols := m.Columns()
	placeholdersForInsert := make([]string, 0, len(cols)-1)
	for _, v := range cols {
		//FIXME: implement PrimaryKey() method, returning a list of column names used
		//together as a primary key constraint.
		if v == "id" {
			continue
		}
		placeholdersForInsert = append(placeholdersForInsert, v)
	}
	return placeholdersForInsert
}

/*
Select prepares and executes a [sqlx.DB.PrepareNamed] and
[sqlx.NamedStmt.Select]. Selected records can be used with [SqlxModel.Data].
`limitAndOffset` is a variadic variable. If passed, it is expected to consist
of two values limit and offset - in that order. The default value  for LIMIT
can be set by [DefaultLimit]. OFFSET is 0 by default.
*/
func (m *Modelx[R]) Select(where string, bindData any, limitAndOffset ...int) ([]R, error) {
	if len(limitAndOffset) == 0 {
		limitAndOffset = append(limitAndOffset, DefaultLimit)
	}
	if len(limitAndOffset) == 1 {
		limitAndOffset = append(limitAndOffset, 0)
	}
	if bindData == nil {
		bindData = struct{}{}
	}
	stash := map[string]any{
		`columns`: strings.Join(m.Columns(), ","),
		`table`:   m.Table(),
		`WHERE`:   where,
		`limit`:   strconv.Itoa(limitAndOffset[0]),
		`offset`:  strconv.Itoa(limitAndOffset[1]),
	}
	query := RenderSQLTemplate(`SELECT`, stash)
	Logger.Debugf("Constructed query : %s", query)
	m.data = make([]R, 0, limitAndOffset[0])
	if stmt, err := DB().PrepareNamed(query); err != nil {
		return nil, fmt.Errorf("error from DB().PrepareNamed(SQL): %w", err)
	} else if err = stmt.Select(&m.data, bindData); err != nil {
		return nil, fmt.Errorf("error from stmt.Select(&m.data, bindData): %w", err)
	}
	return m.data, nil
}

/*
Update constructs a Named UPDATE query and executes it. We assume that the bind
data parameter for [sqlx.DB.NamedExec] is each element of the slice of passed
SqlxRows to [NewModelx].

If exclude is set to true, the parameter `where` is scanned for field names to
exclude fields, used as search criteria in the WHERE clause. The fields, not
used in WHERE clause, are used for the SET clause. The operation is always
performed in a transaction.

For any case in which this method is not suitable, use directly sqlx.
*/
func (m *Modelx[R]) Update(where string, exclude bool) (sql.Result, error) {
	var (
		tx *sqlx.Tx
		r  sql.Result
		e  error
	)
	if tx, e = DB().Beginx(); e != nil {
		return nil, e
	}
	// The rollback will be ignored if the tx has been committed already.
	defer func() { _ = tx.Rollback() }()

	stash := map[string]any{
		`table`: m.Table(),
		// Do not update ID in any case.
		`SET`:   SQLForSET(m.Columns(), where, exclude),
		`WHERE`: where,
	}
	query := RenderSQLTemplate(`UPDATE`, stash)
	Logger.Debugf("Constructed query : %s", query)
	stmt, e := tx.PrepareNamed(query)
	if e != nil {
		return nil, e
	}
	for _, row := range m.data {
		r, e = stmt.Exec(row)
		if e != nil {
			return r, e
		}
	}

	if e := tx.Commit(); e != nil {
		return nil, e
	}
	return r, e
}

/*
Delete deletes records from the database.
*/
func (m *Modelx[R]) Delete(where string, bindData any) (sql.Result, error) {
	stash := map[string]any{
		`table`: m.Table(),
		`WHERE`: where,
	}
	if bindData == nil {
		bindData = map[string]any{}
	}
	query := RenderSQLTemplate(`DELETE`, stash)
	Logger.Debugf("Constructed query : %s", query)
	return DB().NamedExec(query, bindData)
}

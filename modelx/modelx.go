/*
Package modelx provides interfaces and a generic data type, implementing the
interfaces to work easily with database records and sets of records. Underneath
[sqlx] is used. Package modelx provides just an object mapper. The relations'
constraints are left to be managed by the database and you. If you embed
(extend) the data type [Modelx], you get automatically the respective
implementation and can overwrite methods to customise them for your needs.

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
	// Make sure that Modelx implements the full SqlxModel interface.
	_ SqlxModel[alabala] = (*Modelx[alabala])(nil)
)

type alabala struct {
	*Rowx[alabala]
	ID int32
}

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
	singleDB.MapperFunc(CamelToSnakeCase)
	return singleDB
}

/*
SqlxRows is an interface and generic constraint for database records.
*/
type SqlxRows interface {
	// Select one record from its table.
	//	Get(...any) error
	// List of columns which make this record unique.
	PrimaryKeys() []string
	//	Table()
	//	Columns()
}

/*
Rowx is the base type for all structs which represent a database row in a
table. It keeps the metadata retreived from the type it self about table and
columns' names.
*/
type Rowx[R SqlxRows] struct {
	table   string
	columns []string
}

// PrimaryKeys returns a slice of one element {`id`}. The purpose is to have an
// overwritable default for types, which embed Rowx[R].
func (r *Rowx[R]) PrimaryKeys() []string {
	return []string{`id`}
}

// Table returns the guessed table name from the type parameter of the record.
func (r *Rowx[R]) Table() string {
	if r.table != "" {
		return r.table
	}
	r.table = TypeToSnakeCase(new(R))
	return r.table
}

/*
Columns returns a slice with the names of the table's columns in no particular
order.
*/
func (r *Rowx[R]) Columns() []string {
	if len(r.columns) > 0 {
		return r.columns
	}
	r.columns = columns[R]()
	return r.columns
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
	Update([]string, string) (sql.Result, error)
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
Modelx implements the [SqlxModel] interface and can be used right away or
embedded (extended) to customise its behaviour for your own needs.

Direct usage:

	type Users struct {
		ID        int32
		LoginName string
		// ...
	}

	var users = []Users{
		Users{LoginName: "first"},
		Users{LoginName: "the_second"},
		Users{LoginName: "the_third"},
	}

	m := modelx.NewModel[Users](users)
	r, e := m.Insert()
	if e != nil {
		fmt.Fprintf(os.Stderr, "Got error from m.Insert(): %s", e.Error())
		return
	}

To embed this type, write something similar to the following:

	// MyModel requires that the records implement the SQLxRows interface.
	type MyModel[R SQLxRows] struct {
		modelx.Modelx[R]
		data []R
	}
	// Data returns the collected data from a Select or provided objects.
	func (m *MyModel[R]) Data() []R {
		return m.data
	}
	// Now you can implement some custom methods to insert, select, update and
	// delete your sets of records. Maybe some custom constructor.
	//...
	// Somewhere else in the code, using your class...
	mm = new(myModel[Users])
	// WHERE clause can be as complex as you need.
	data, err := mm.Select(`WHERE id >:id`, map[string]any{`id`: 1}.
	// And you can implement your own Columns() and Table()...
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

// Table returns the guessed table name from the type parameter.
func (m *Modelx[R]) Table() string {
	if m.table != "" {
		return m.table
	}
	m.table = TypeToSnakeCase(new(R))
	return m.table
}

// Data returns the slice of structs, passed to [NewModel]. It may return nil
// if no rows are passed.
func (m *Modelx[R]) Data() []R {
	return m.data
}

/*
Columns returns a slice with the names of the table's columns in no particular
order.
*/
func (m *Modelx[R]) Columns() []string {
	if len(m.columns) > 0 {
		return m.columns
	}
	m.columns = columns[R]()
	return m.columns
}

func columns[R SqlxRows]() []string {
	/*
		TODO: Some day... use go:generate to move such code to compile time for
		SqlxRows implementing types. Consider also a solution to (eventually
		gradually) regenerate Rowx embedding types and recompile the application
		due to changes in the database schema. This is how we can implement
		database migrations starting from the database.
		1. During development the owner of the user code changes the development
		database and runs `go generate && go build -ldflags "-s -w" ./...` to
		(re-)generate types which will embed Rowx. Then recompiles the
		application.
		2. Next he/she prepares the sql migration file to be run on the
		production database. It should not be harmfull if some defined fields in
		the Rowx embedding types do not have corresponding columns in the
		database, because they will have sane defaults, thanks to Go default
		values. Also it will not harm if there are new columns in tables and
		some types do not have yet the corresponding field. The only problematic
		case is when a column in the database changes its type or a table is
		dropped. To cover this case...
		3. Deployment.
			a. Dabase migration trough executing the prepared SQL file.
			b. Disallow requests by showing a static page(Maintenance time -
			this should take less than a second).
			b. Imediately deploy the static binary. If it is a CGI application,
			on the next request the updated binary will run. If it is a running
			application (a (web-)service), immediately restart the application.
		Consider carefully!:
		https://stackoverflow.com/questions/55934210/creating-structs-programmatically-at-runtime-possible
		https://agirlamonggeeks.com/golang-dynamic-lly-generate-struct/
	*/
	colMap := DB().Mapper.TypeMap(reflect.ValueOf(new(R)).Type()).Names
	columns := make([]string, 0, len(colMap))
	for k := range colMap {
		if strings.Contains(k, `.`) {
			continue
		}
		columns = append(columns, k)
	}
	Logger.Debugf(`columns: %#v`, columns)
	return columns
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
	Logger.Debugf("Rendered query: %s", query)
	if dataLen > 1 {
		var (
			tx *sqlx.Tx
			r  sql.Result
			e  error
		)
		tx = DB().MustBegin()
		// The rollback will be ignored if the tx has been committed already.
		defer func() { _ = tx.Rollback() }()
		for _, row := range m.data {
			r, e = tx.NamedExec(query, row)
			if e != nil {
				return r, e
			}
		}
		if e = tx.Commit(); e != nil {
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
Select prepares and executes a SELECT statement. Selected records can be used
with [SqlxModel.Data].`limitAndOffset` is expected to be used as a variadic
parameter. If passed, it is expected to consist of two values limit and offset
- in that order. The default value  for LIMIT can be set by [DefaultLimit].
OFFSET is 0 by default.
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
	Logger.Debugf("Rendered query : %s", query)
	m.data = make([]R, 0, limitAndOffset[0])

	q, args, err := sqlx.Named(query, bindData)
	if err != nil {
		return nil, err
	}
	q, args, err = sqlx.In(q, args...)
	if err != nil {
		return nil, err
	}
	q = DB().Rebind(q)
	if err := DB().Select(&m.data, q, args...); err != nil {
		Logger.Debugf("Select q :'%s', args:'%#v', err:'%#v'", query, args, err)
		return nil, err
	}
	//	if stmt, err := DB().PrepareNamed(query); err != nil {
	//		return nil, fmt.Errorf("error from DB().PrepareNamed(SQL): %w", err)
	//	} else if err = stmt.Select(&m.data, bindData); err != nil {
	//		return nil, fmt.Errorf("error from stmt.Select(&m.data, bindData): %w", err)
	//	}
	return m.data, nil
}

/*
Update constructs a Named UPDATE query and executes it. We assume that the bind
data parameter for [sqlx.DB.NamedExec] is each element of the slice of passed
SqlxRows to [NewModelx].

`fields` is the list of columns to be updated - used in `SET col = :col...`. If
a field starts with UppercaseLetter it is converted to snake_case.

For any case in which this method is not suitable, use directly sqlx.
*/
func (m *Modelx[R]) Update(fields []string, where string) (sql.Result, error) {
	var (
		tx *sqlx.Tx
		r  sql.Result
		e  error
	)
	tx = DB().MustBegin()
	// The rollback will be ignored if the tx has been committed already.
	defer func() { _ = tx.Rollback() }()

	stash := map[string]any{
		`table`: m.Table(),
		// Do not update ID in any case.
		`SET`:   SQLForSET(fields),
		`WHERE`: where,
	}
	query := RenderSQLTemplate(`UPDATE`, stash)
	Logger.Debugf("Rendered query : %s", query)
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

	if e = tx.Commit(); e != nil {
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

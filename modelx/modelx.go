/*
Package modelx is a minimalistic object to database-rows mapper and wrapper for
[sqlx]. It provides interfaces and a generic data type, and implements the
provided interfaces to work easily with sets of database records. The
relations' constraints are left to be managed by the database and you. This may
be improved in a future release.

By default the current implementation assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. You can mark such fields with tags. See below.

# Synopsis

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

	r, e := modelx.NewModelx(users).Insert()
	if e != nil {
		fmt.Fprintf(os.Stderr, "Got error from m.Insert(): %s", e.Error())
		return
	}
	//...
*/
package modelx

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/labstack/gommon/log"
)

var (
	// DefaultLogHeader is a template for modelx logging
	DefaultLogHeader = `${prefix}:${level}:${short_file}:${line}`
	// DefaultLimit is the default LIMIT for SQL queries.
	DefaultLimit = 100
	// DriverName is the name of the database engine to use. It is set by
	// default to `sqlite3`.
	DriverName = `sqlite3`
	// DSN must be set before using DB() function. It is set by default to `:memory:`.
	DSN = `:memory:`
	// Logger is instantiated (if not instantiated already externally) during
	// first call of DB() and the log level is set to log.DEBUG.
	Logger *log.Logger
	// ReflectXTag sets the tag name for identifying tags, read and acted upon
	// by sqlx and Modelx.
	ReflectXTag = `rx`
	// singleDB is a singleton for the connection pool to the database.
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
	singleDB.Mapper = reflectx.NewMapperFunc(ReflectXTag, CamelToSnakeCase)
	return singleDB
}

/*
SqlxRows is an empty interface and generic constraint for database records.
Any struct type implements it.
*/
type SqlxRows interface {
}

/*
SqlxModel is an interface and generic constraint for working with a set of
database records. [Modelx] fully implements SqlxModel. You can embed (extend)
Modelx to get automatically its implementation and override some of its
methods.
*/
type SqlxModel[R SqlxRows] interface {
	SetData([]R) SqlxModel[R]
	SqlxModelInserter[R]
	SqlxModelSelector[R]
	SqlxModelUpdater[R]
	SqlxModelDeleter[R]
}

/*
SqlxModelInserter can be implemented to insert records in a table. It is fully
implemented by [Modelx].
*/
type SqlxModelInserter[R SqlxRows] interface {
	Data() []R
	Table() string
	Columns() []string
	Insert() (sql.Result, error)
}

/*
SqlxModelUpdater can be implemented to update records in a table. It is fully
implemented by [Modelx].
*/
type SqlxModelUpdater[R SqlxRows] interface {
	Data() []R
	Table() string
	Update([]string, string) (sql.Result, error)
}

/*
SqlxModelGetter can be implemented to get one record from the database. It is
fully implemented by [Modelx].
*/
type SqlxModelGetter[R SqlxRows] interface {
	Table() string
	Columns() []string
	Get(string, ...any) (*R, error)
}

/*
SqlxModelSelector can be implemented to select records from a table or view. It
is fully implemented by [Modelx].
*/
type SqlxModelSelector[R SqlxRows] interface {
	SqlxModelGetter[R]
	Select(string, any, ...int) ([]R, error)
}

/*
SqlxModelDeleter can be implemented to delete records from a table. It is
fully implemented by [Modelx].
*/
type SqlxModelDeleter[R SqlxRows] interface {
	Table() string
	Delete(string, any) (sql.Result, error)
}

/*
Modelx implements the [SqlxModel] interface and can be used right away or
embedded (extended) to customise its behaviour for your own needs.

To embed this type, write something similar to the following:

	type MyTableName struct {
		modelx.Modelx[MyTableName]
		data []MyTableName
	}
	// Now you can implement some custom methods to insert, select, update and
	// delete your sets of records. Maybe some custom constructor.
	//...
	// Somewhere else in the code, using it...
	mm = new(MyTableName)
	// WHERE clause can be as complex as you need.
	data, err := mm.Select(`WHERE id >:id`, map[string]any{`id`: 1}.
	// And you may want to implement your own Columns() and Table()...
*/
type Modelx[R SqlxRows] struct {
	// structMap is an index of field metadata for the underlying struct R.
	structMap *reflectx.StructMap
	/*
		table allows to set explicitly the table name for this model. Otherwise
		it is guessed and set from the type of the first element of Data slice
		upon first use of '.Table()'.
	*/
	table string
	// columns of the table are populated upon first use of '.Columns()'.
	columns []string
	/*
		data is a slice of rows, retrieved from the database or to be inserted,
		or updated.
	*/
	data []R
}

/*
NewModelx returns a new instance of a table model with optionally provided data
rows as a variadic parameter. Providing the specific type parameter to
instantiate is mandatory if it cannot be inferred from the variadic parameter.
*/
func NewModelx[R SqlxRows](rows ...R) SqlxModel[R] {
	m := &Modelx[R]{data: rows}
	return m
}

// rowx returns a (*R)(nil). We use it only for metadata extraction. So we do
// not need to allocate any memory.
func (m *Modelx[R]) rowx() *R {
	return (*R)(nil)
}

// fieldsMap returns a pointer to [reflectx.structMap] for the generic
// structure. It is the cornerstone to implement the SqlxModelMeta interface.
func (m *Modelx[R]) fieldsMap() *reflectx.StructMap {
	if m.structMap != nil {
		return m.structMap
	}
	m.structMap = DB().Mapper.TypeMap(reflect.ValueOf(m.rowx()).Type())
	return m.structMap
}

// Table returns the converted to snake case name of the type to be used as
// table name in sql queries.
func (m *Modelx[R]) Table() string {
	if m.table != "" {
		return m.table
	}
	m.table = TypeToSnakeCase(m.rowx())
	return m.table
}

/*
Data returns the slice of structs, passed to [NewModelx]. It may return nil
if no rows were passed to [NewModelx].
*/
func (m *Modelx[R]) Data() []R {
	return m.data
}

/*
SetData sets a slice of R to be inserted or updated in the database.
*/
func (m *Modelx[R]) SetData(data []R) SqlxModel[R] {
	m.data = data
	return m
}

/*
Columns returns a slice with the names of the table's columns.
*/
func (m *Modelx[R]) Columns() []string {
	if len(m.columns) > 0 {
		return m.columns
	}
	/*
	   TODO: Some day... use go:generate to move such code to compile time for
	   SqlxRows implementing types. Consider also a solution to (eventually
	   gradually) regenerate Modelx embedding types and recompile the
	   application due to changes in the database schema. This is how we can
	   implement database migrations starting from the database.  1. During
	   development the owner of the user code changes the development database
	   and runs `go generate && go build -ldflags "-s -w" ./...` to
	   (re-)generate types which may need to embed Modelx. Then recompiles the
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
	colMap := m.fieldsMap().Names
	m.columns = make([]string, 0, len(colMap))
	for k, v := range colMap {
		// Logger.Debugf("column: %s, Name: %v; Tag: %#v; Options: %#v; Path: %v", k, v.Field.Name, v.Field.Tag, v.Options, v.Path)
		if _, exists := v.Options[`-`]; exists {
			Logger.Debugf("Skipping field %s; Options %v", v.Field.Name, v.Options)
			continue
		}
		// Nested fields are not columns either. They are used by sqlx for other purposes.
		if strings.Contains(k, `.`) {
			continue
		}
		m.columns = append(m.columns, k)
	}
	Logger.Debugf(`columns: %#v`, m.columns)

	return m.columns
}

/*
Insert inserts a set of SqlxRows instances (without their primary key values) and
returns [sql.Result] and [error]. The value for the autoincremented primary key
(usually ID column) is left to be set by the database.

If the records to be inserted are more than one, the data is inserted in a
transaction. [sql.Result.RowsAffected] will always return 1, because every row
is inserted in its own statement. This may change in a future release. If there
are no records to be inserted, [Modelx.Insert] panics.

If you need to insert an [SqlxRows] structure with a specific value for ID, add a
tag to the ID column `rx:id,no_auto` or use directly [sqlx].

If you want to skip any field during insert add, a tag to it `rx:field_name,auto`.
*/
func (m *Modelx[R]) Insert() (sql.Result, error) {
	dataLen := len(m.Data())
	if dataLen == 0 {
		Logger.Panic("Cannot insert, when no data is provided!")
	}
	// TODO: Think of caching noAutoColumns (and use go:generate for all metadata)
	noAutoColumns := make([]string, 0, len(m.Columns())-1)
	names := m.fieldsMap().Names
	for _, col := range m.Columns() {
		// insert column named ID but with tag option no_auto: `rx:"id,no_auto"`
		if _, isNoAuto := names[col].Options[`no_auto`]; col == `id` && isNoAuto {
			continue
		}
		// do not insert collumns with tag `auto`
		if _, ok := names[col].Options[`auto`]; ok {
			continue
		}
		noAutoColumns = append(noAutoColumns, col)
	}
	placeholders := strings.Join(noAutoColumns, ",:") // :login_name,:changed_by...
	placeholders = sprintf("(:%s)", placeholders)
	// END TODO
	stash := map[string]any{
		`columns`:      strings.Join(noAutoColumns, ","),
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
		for _, row := range m.Data() {
			// Logger.Debugf("Inserting row: %+v", row)
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

/*
Select prepares, executes a SELECT statement and returns the collected result
as a slice. Selected records can also be used with [Modelx.Data].
`limitAndOffset` is expected to be used as a variadic parameter. If passed, it
is expected to consist of two values limit and offset - in that order. The
default value for LIMIT can be set by [DefaultLimit]. OFFSET is 0 by default.
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
	query := m.renderSelectTemplate(where, limitAndOffset)
	m.data = make([]R, 1, limitAndOffset[0])

	q, args, err := namedInRebind(query, bindData)
	if err != nil {
		return nil, err
	}
	if err := DB().Select(&m.data, q, args...); err != nil {
		Logger.Debugf("Select q :'%s', args:'%#v', err:'%#v'", query, args, err)
		return m.data, err
	}
	//	if stmt, err := DB().PrepareNamed(query); err != nil {
	//		return nil, fmt.Errorf("error from DB().PrepareNamed(SQL): %w", err)
	//	} else if err = stmt.Select(&m.data, bindData); err != nil {
	//		return nil, fmt.Errorf("error from stmt.Select(&m.data, bindData): %w", err)
	//	}
	return m.data, nil
}

func (m *Modelx[R]) renderSelectTemplate(where string, limitAndOffset []int) string {
	stash := map[string]any{
		`columns`: strings.Join(m.Columns(), ","),
		`table`:   m.Table(),
		`WHERE`:   ifWhere(where),
		`limit`:   strconv.Itoa(limitAndOffset[0]),
		`offset`:  strconv.Itoa(limitAndOffset[1]),
	}
	query := RenderSQLTemplate(`SELECT`, stash)
	Logger.Debugf("Rendered SELECT query : %s", query)
	return query
}

/*
Get executes [sqlx.DB.Get] and returns the result scanned into an instantiated
[SqlxRows] object or an error.
*/
func (m *Modelx[R]) Get(where string, bindData ...any) (*R, error) {
	row := new(R)
	query := m.renderSelectTemplate(where, []int{1, 0})
	var (
		q    string
		args []any
		err  error
	)
	if len(bindData) == 0 {
		bindData = append(bindData, struct{}{})
	}
	q, args, err = namedInRebind(query, bindData[0])
	if err != nil {
		return row, err

	}
	return row, DB().Get(row, q, args...)
}

var isWhere = regexp.MustCompile(`(?i:^\s*?where\s)`)

func ifWhere(where string) string {
	if where != `` && !isWhere.MatchString(where) {
		where = sprintf(`WHERE %s`, where)
	}
	return where
}

func namedInRebind(query string, bindData any) (string, []any, error) {
	q, args, err := sqlx.Named(query, bindData)
	if err != nil {
		return query, args, err
	}
	q, args, err = sqlx.In(q, args...)
	if err != nil {
		return query, args, err
	}
	q = DB().Rebind(q)
	Logger.Debugf(`Rebound query: %s|args:%+v| err: %+v`, q, args, err)
	return q, args, err
}

/*
Update constructs a Named UPDATE query, prepares it and executes it for each
row of data in a transaction. It panics if there is no data to be updated.

We pass as bind parameters for each [sqlx.NamedStmt.Exec] each element
of the slice of passed [SqlxRows] to [NewModelx] or to [Modelx.SetData].

This is somehow problematic with named queries. What if we want to `SET
group_id=1 WHERE group_id=2. How to differntiate between columns to be updated
and parameters for the WHERE clause?  We need different name for the bind
parameter. Something like `:where.group_id` to hold the existing value in the
database. Or maybe use a nested select statement in the WHERE clause to match
the needed row for update by primary key column. A solution is to have a nested
structure in the passed record, used only as parameters for the query.
We can enrich our structure, representing the database record with a `Where`
field which is a structure and holds the current values. Look in the tests for
an example of updating such an enriched record. Also we can use for our
columns types like [sql.NullInt32] and such, provided by the [sql] package.

`fields` is the list of columns to be updated - used to construct the `SET col
= :col...` part of the query. If a field starts with UppercaseLetter it is
converted to snake_case.

For any case in which this method is not suitable, use directly sqlx.
*/
func (m *Modelx[R]) Update(fields []string, where string) (sql.Result, error) {
	if len(m.Data()) == 0 {
		Logger.Panic("Cannot update, when no data is provided!")
	}
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
		// TODO: Prevent updating AutoFields in any case.
		`SET`:   SQLForSET(fields),
		`WHERE`: ifWhere(where),
	}
	query := RenderSQLTemplate(`UPDATE`, stash)
	Logger.Debugf("Rendered UPDATE query : %s;", query)
	namedStmt, e := tx.PrepareNamed(query)
	if e != nil {
		return nil, e
	}
	for _, row := range m.Data() {
		Logger.Debugf("Update row: %+v;", row)
		r, e = namedStmt.Exec(row)
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
		`WHERE`: ifWhere(where),
	}
	if bindData == nil {
		bindData = map[string]any{}
	}
	query := RenderSQLTemplate(`DELETE`, stash)
	Logger.Debugf("Constructed query : %s", query)
	return DB().NamedExec(query, bindData)
}

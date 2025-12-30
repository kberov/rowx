/*
Package rx provides a minimalistic objects to table-rows mapper by using the
scanning capabilities of [sqlx]. It is also an SQL builder using SQL templates.

At runtime the templates get filled in with metadata (tables' and columns'
names from the provided data structures) and WHERE clauses, written by you -
the programmer in SQL. The rendered by [fasttemplate] SQL query is prepared and
executed by [sqlx].

In other words, package `rx` provides functions, interfaces and a generic data
type [Rx], which wraps data structures. The structs may be provided by you or
generated from existing tables by [Generate]. [Rx] implements the provided
interfaces to execute CRUD operations. The relations' constraints are left to
be managed by the database.

To ease schema migrations, `rx` provides two functions - [Migrate] and
[Generate]. The first executes sets of SQL statements from a file to migrate
the the database schema to a new state and the second re-generates the structs,
mappped to rows in tables.

By default the current implementation assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. You can mark such fields with tags.

# Synopsis

	// Have an existing or newly created database. Generate a model package
	// from it using the companion commandline tool `rowx`.
	cd to/your/project/root
	// make a directory for your package, named for example "model"
	mkdir -p internal/example/model
	// Generate structures from all tables in the database, implementing
	// SqlxMeta interface.
	rowx generate -dsn /some/path/mydb-development.sqlite -package ./internal/example/model

	// Use the structures in your application.
	// ...
	// Have a structure, mapping a table row, generated in
	// ./internal/example/model/model_tables.go.
	type Users struct {
		LoginName string
		// ...
		ID        int64
	}

	// Have a slice of Users to Insert.
	var users = []Users{
		Users{LoginName: "first"},
		Users{LoginName: "the_second"},
		Users{LoginName: "the_third"},
	}

	// Insert them.
	r, e := rx.NewRx(users).Insert()
	if e != nil {
		fmt.Fprintf(os.Stderr, "Got error from m.Insert(): %s", e.Error())
		return
	}
	//... time passes

	// Create a new migration file or add to an existing one a new set of SQL
	// statements to migrate the database to a new state.
	cd to/your/project/root
	vim data/migrations_01.sql
	// Migrate.
	./rowx migrate -sql_file data/migrations_01.sql -dsn=/tmp/test.sqlite -direction=up
	// Run generate again to reflect the changes in the schema.
	rowx generate -dsn /some/path/mydb-development.sqlite -package ./internal/example/model
	// Edit your code, which uses the structures, if needed.
	// During deployment just run `rowx migrate` again on the production
	// datatbase.
	// ...and so the life of the application continues further on.

[sqlx]: https://github.com/jmoiron/sqlx
[fasttemplate]: https://github.com/valyala/fasttemplate
*/
package rx

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
	_ "github.com/mattn/go-sqlite3" //no-lint:revive
)

const (
	// DefaultLimit is the default LIMIT for SQL queries.
	DefaultLimit = 100
	// DriverName is the name of the database engine to use. For now we only
	// support `sqlite3`. Support for PostreSQL and MySQL is planned.
	DriverName = `sqlite3`
	// MigrationsTable is where we keep information about executed schema
	// migrations.
	MigrationsTable = `rx_migrations`
)

var (
	// DefaultLogHeader is a template for rx logging.
	DefaultLogHeader = `${prefix}:${level}:${short_file}:${line}`
	// DefaultLogOutput is where the output from the Logger will go to.
	DefaultLogOutput = os.Stderr
	// DSN must be set before using DB() function. It is set by default to
	// `:memory:`, because the default DriverName = `sqlite3`. See also options
	// for the connection string when using sqlite3:
	// https://github.com/mattn/go-sqlite3?tab=readme-ov-file#connection-string
	// .
	DSN = `:memory:`
	// Logger is always instantiated and the log level is set to log.DEBUG. You
	// can change the log level as you wish. We use
	// `github.com/labstack/gommon/log` as logging engine.
	Logger = newLogger()
	// ReflectXTag sets the tag name for identifying tags, read and acted upon
	// by sqlx and Rx.
	ReflectXTag = `rx`
	// singleDB is a singleton for the connection pool to the database.
	singleDB *sqlx.DB
	sprintf  = fmt.Sprintf
)

func newLogger() (l *log.Logger) {
	l = log.New(ReflectXTag)
	l.SetOutput(DefaultLogOutput)
	l.SetHeader(DefaultLogHeader)
	l.SetLevel(log.DEBUG)
	return
}

/*
DB invokes [sqlx.MustConnect] and assigns the returned [sqlx.DB] pointer to a
private package variable, if not assigned already. This private variable is
simply returned on subsequent calls. Then DB sets the [sqlx.DB.Mapper], using
[ReflectXTag], and [CamelToSnake] as parameters to [reflectx.NewMapperFunc].

[sqlx.DB] is a wrapper around [sql.DB]. A DB instance is not a connection, but
an abstraction representing a Database. This is why creating a *sqlx.DB does
not return an error and will not panic. It maintains a connection pool
internally, and will attempt to connect when a connection is first needed.
*/
func DB() *sqlx.DB {
	if singleDB != nil {
		return singleDB
	}
	Logger.Debugf("Connecting to database '%s'...", DSN)

	singleDB = sqlx.MustConnect(DriverName, DSN)
	singleDB.Mapper = reflectx.NewMapperFunc(ReflectXTag, CamelToSnake)
	return singleDB
}

/*
ResetDB closes the connection to the database and undefines the underlying
variable, holding the connection.
*/
func ResetDB() {
	if singleDB == nil {
		return
	}
	if err := singleDB.Close(); err != nil {
		Logger.Errorf(`connection closed unsuccesfully: %s`, err.Error())
	}
	singleDB = nil
}

// Ext is a unified constraint for *sqlx.Tx and *sqlx.DB.
type Ext interface {
	sqlx.Ext
	PrepareNamed(query string) (*sqlx.NamedStmt, error)
}

/*
Rx implements the [SqlxModel] interface and can be used right away or
embedded (extended) to override some methods for a struct or set of structs.
*/
type Rx[R Rowx] struct {
	// An instance of R which may implement the SqlxMeta interface (even only
	// partially).
	r *R
	/*
		data is a slice of rows, retrieved from the database or to be inserted,
		or updated.
	*/
	data []R
	/*
		table allows to set explicitly the table name of this record. Otherwise
		it is guessed and set from the type of the first element of data slice in Rx[R]
		upon first use of '.Table()'.
	*/
	table string
	// columns of the table are populated upon first use of '.Columns()'.
	columns []string
	queryer Ext
}

/*
NewRx returns a new instance of a table model with optionally provided data
rows as a variadic parameter. Providing the specific type parameter to
instantiate is mandatory if a variadic parameter is not passed.
*/
func NewRx[R Rowx](rows ...R) SqlxModel[R] {
	return &Rx[R]{data: rows, r: nilRowx[R]()}
}

// Tx returns an *sqlx.DB or *sqlx.Tx.
func (m *Rx[R]) Tx() Ext {
	if m.queryer != nil {
		return m.queryer
	}
	return DB()
}

// WithTx allows you to set an [sqlx.Tx] to be shared among several objects
// to execute several SQL statements in one transaction.
func (m *Rx[R]) WithTx(queryer Ext) SqlxModel[R] {
	m.queryer = queryer
	return m
}

/*
nilRowx returns a (*R)(nil). [Rx] uses it only for metadata extraction. So it
does not need to allocate any memory. If a [Rowx] structure implements
[SqlxMeta], it may need to be instantiated. [Rx] does that only if it finds
that the generic structure implements [SqlxMeta] at least partially. See
[Columns] and [Table].
*/
func nilRowx[R Rowx]() *R {
	return (*R)(nil)
}

/*
fieldsMap returns a pointer to an instantiated and cached [reflectx.StructMap]
for the generic structure. It is used to scan the tags of the fields and get
column names and tag options.

We implemented Migrate and Generate but still it worths it to consider carefully!:
https://stackoverflow.com/questions/55934210/creating-structs-programmatically-at-runtime-possible
https://agirlamonggeeks.com/golang-dynamic-lly-generate-struct/
*/
func fieldsMap[R Rowx]() *reflectx.StructMap {
	return DB().Mapper.TypeMap(reflect.ValueOf(nilRowx[R]()).Type())
}

/*
Table returns the converted to snake_case name of the type to be used as table
name in sql queries. If the underlying type implements the method Table from
[SqlxMeta], the type is instantiated (if not already) and the method is called.
*/
func (m *Rx[R]) Table() string {
	if m.table != "" {
		return m.table
	}
	/*
		An implementing (at least partially) SqlxMeta type and not implementing
		SqlxModel (Rowx(m.r).(SqlxModel[R]) == embeds Rx), because if the
		underlying structure embeds Rx, we end up with stackoverflow (because
		each next call enters this if, causing endelss recursion).
	*/
	if _, ok := Rowx(m.r).(SqlxModel[R]); !ok {
		if _, ok = Rowx(m.r).(interface{ Table() string }); ok {
			if m.r == nilRowx[R]() {
				Logger.Debugf("Instantiating %#v...", m.r)
				m.r = new(R)
			}
			m.table = Rowx(m.r).(interface{ Table() string }).Table()
			return m.table
		}
	}
	m.table = TypeToSnake(nilRowx[R]())
	return m.table
}

/*
Data returns the slice of structs, passed to [NewRx] or selected from the
database. It may return nil if no rows were passed to [NewRx].
*/
func (m *Rx[R]) Data() []R {
	return m.data
}

/*
SetData sets a slice of R to be inserted or updated in the database. Returns
the current instance of [Rx].
*/
func (m *Rx[R]) SetData(data []R) SqlxModel[R] {
	m.data = data
	return m
}

/*
Columns returns a slice with the names of the table's columns. If the underlying
type implements the method Columns from [SqlxMeta], the type is instantiated
(if not already) and the method is called.
*/
func (m *Rx[R]) Columns() []string {
	if len(m.columns) > 0 {
		return m.columns
	}
	/*
		An implementing (at least partially) SqlxMeta type and not implementing
		SqlxModel (== embeds Rx), because if the underlying structure embeds Rx, we
		end up with stackoverflow (because each next call enters this if,
		causing endelss recursion).
	*/
	if _, ok := Rowx(m.r).(SqlxModel[R]); !ok {
		if _, ok = Rowx(m.r).(interface{ Columns() []string }); ok {
			if m.r == nilRowx[R]() {
				Logger.Debugf("Instantiating %#v...", m.r)
				m.r = new(R)
			}
			m.columns = Rowx(m.r).(interface{ Columns() []string }).Columns()
			return m.columns
		}
	}

	colIndex := fieldsMap[R]().Index
	m.columns = make([]string, 0, len(colIndex))
	for _, v := range colIndex {
		//		Logger.Debugf("column: %s, Field.Name: %v; Field.Tag: %#v; Options: %#v; Path: %v",
		//			v.Name, v.Field.Name, v.Field.Tag, v.Options, v.Path)
		// Skip Rx in case this struct embeds it
		if v.Name == `rx` {
			continue
		}
		if _, exists := v.Options[`-`]; exists {
			Logger.Debugf("Skipping field %s; Options %v", v.Field.Name, v.Options)
			continue
		}
		// Nested fields are not columns either. They are used for other purposes.
		if strings.Contains(v.Path, `.`) {
			continue
		}
		m.columns = append(m.columns, v.Path)
	}
	Logger.Debugf(`columns: %#v`, m.columns)

	return m.columns
}

/*
Insert inserts a slice of Rowx instances (without their primary key values) and
returns [sql.Result] and [error]. The value for the autoincremented primary key
(usually ID column) is left to be set by the database.

If the records to be inserted are more than one, the data is inserted in a
transaction. [sql.Result.RowsAffected] will always return 1, because every row
is inserted in its own statement. This may change in a future release. If there
are no records to be inserted, [Rx.Insert] panics.

If you need to insert a [Rowx] structure with a specific value for ID, add a
tag to the ID column `rx:"id,no_auto"` or use directly [sqlx].

If you want to skip any field during insert (including `id`) add, a tag to it
`rx:"field_name,auto"`.
*/
func (m *Rx[R]) Insert() (sql.Result, error) {
	if len(m.Data()) == 0 {
		Logger.Panic("Cannot insert, when no data is provided!")
	}
	query := m.renderInsertQuery()
	Logger.Debugf("Rendered query: %s", query)
	Logger.Debugf("Inserting rows: %+v", m.Data())
	return sqlx.NamedExec(m.Tx(), query, m.Data())
}

func (m *Rx[R]) renderInsertQuery() string {
	// TODO: Think of caching noAutoColumns (and use go:generate for all metadata)
	noAutoColumns := make([]string, 0, len(m.Columns())-1)
	names := fieldsMap[R]().Names
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
		`columns`: strings.Join(noAutoColumns, ","),
		`table`:   m.Table(),
		// TODO:
		// `placeholders`: strings.TrimSuffix(strings.Repeat(placeholders+`,`, dataLen), `,`),
		`placeholders`: placeholders,
	}
	query := RenderSQLTemplate(`INSERT`, stash)
	return query
}

/*
Select prepares, executes a SELECT statement and returns the collected result
as a slice. Selected records can also be used with [Rx.Data].

  - `where` is expected to contain the `WHERE` clause with potentially subsequent
    `ORDER BY` clause. the keyword `WHERE` can be omitted.
  - `bindData` can be a struct (even unnamed) or map[string]any.
  - `limitAndOffset` is expected to be used as a variadic parameter. If passed,
    it is expected to consist of two values limit and offset - in that order. The
    default value for LIMIT can be set by [DefaultLimit]. OFFSET is 0 by default.
*/
func (m *Rx[R]) Select(where string, bindData any, limitAndOffset ...int) ([]R, error) {
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
	return m.data, sqlx.Select(m.Tx(), &m.data, q, args...)
}

func (m *Rx[R]) renderSelectTemplate(where string, limitAndOffset []int) string {
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
[Rowx] object or an error.
*/
func (m *Rx[R]) Get(where string, bindData ...any) (*R, error) {
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
		return nilRowx[R](), err
	}
	m.r = new(R)
	return m.r, sqlx.Get(m.Tx(), m.r, q, args...)
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
of the slice of passed [Rowx] to [NewRx] or to [Rx.SetData].

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
func (m *Rx[R]) Update(fields []string, where string) (sql.Result, error) {
	if len(m.Data()) == 0 {
		Logger.Panic("Cannot update, when no data is provided!")
	}
	var (
		r sql.Result
		e error
	)

	stash := map[string]any{
		`table`: m.Table(),
		// TODO: Prevent updating AutoFields in any case.
		`SET`:   SQLForSET(fields),
		`WHERE`: ifWhere(where),
	}
	query := RenderSQLTemplate(`UPDATE`, stash)
	Logger.Debugf("Rendered UPDATE query : %s;", query)
	namedStmt, e := m.Tx().PrepareNamed(query)
	if e != nil {
		return nil, e
	}
	defer func() { _ = namedStmt.Close() }()
	for _, row := range m.Data() {
		Logger.Debugf("Update row: %+v;", row)
		r, e = namedStmt.Exec(row)
		if e != nil {
			return r, e
		}
	}

	return r, e
}

/*
Delete deletes records from the database.
*/
func (m *Rx[R]) Delete(where string, bindData any) (sql.Result, error) {
	stash := map[string]any{
		`table`: m.Table(),
		`WHERE`: ifWhere(where),
	}
	if bindData == nil {
		bindData = map[string]any{}
	}
	query := RenderSQLTemplate(`DELETE`, stash)
	Logger.Debugf("Constructed DELETE query : %s", query)

	return sqlx.NamedExec(m.Tx(), query, bindData)
}

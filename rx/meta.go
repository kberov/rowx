package rx

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

/*
Rowx is an empty interface and generic constraint for database records.
Any struct type implements it. The fields of such a struct are expected to
map to cells (columns) of a row in a database table.
*/
type Rowx interface{}

/*
SqlxModel is an interface and generic constraint for working with a set of
database records. [Rx] fully implements SqlxModel. You can embed (extend)
Rx to get automatically its implementation and override some of its
methods.
*/
type SqlxModel[R Rowx] interface {
	Data() []R
	SetData(data []R) (rx SqlxModel[R])
	SqlxDeleter[R]
	SqlxGetter[R]
	SqlxInserter[R]
	SqlxMeta[R]
	SqlxSelector[R]
	SqlxUpdater[R]
	Tx() *sqlx.Tx
	WithTx(queryer *sqlx.Tx) SqlxModel[R]
}

/*
SqlxInserter can be implemented to insert records in a table. It is fully
implemented by [Rx].
*/
type SqlxInserter[R Rowx] interface {
	/*
	   Insert inserts a set of Rowx instances (without their primary key values) and
	   returns [sql.Result] and [error]. The value for the autoincremented primary key
	   (usually ID column) is left to be set by the database.
	*/
	Insert() (sql.Result, error)
}

/*
SqlxUpdater can be implemented to update records in a table. It is fully
implemented by [Rx].
*/
type SqlxUpdater[R Rowx] interface {
	Update(fields []string, where string) (sql.Result, error)
}

/*
SqlxGetter can be implemented to get one record from the database. It is
fully implemented by [Rx].
*/
type SqlxGetter[R Rowx] interface {
	/*
		Get expects a string to be used as where clause and optional bindata
		(struct or map[string]any).
	*/
	Get(where string, binData ...any) (*R, error)
}

/*
SqlxSelector can be implemented to select records from a table or view. It
is fully implemented by [Rx].
*/
type SqlxSelector[R Rowx] interface {
	Select(where string, binData any, limitAndOffset ...int) ([]R, error)
}

/*
SqlxDeleter can be implemented to delete records from a table. It is
fully implemented by [Rx].
*/
type SqlxDeleter[R Rowx] interface {
	Delete(where string, binData any) (sql.Result, error)
}

/*
SqlxMeta can be implemented to return the name of the table in the database for
the implementing type and the slice with its column names. It is fully
implemented by [Rx].

If you implement this interface for a struct, its methods will be called by
[Rx] everywhere where table name or a slice of columns are needed. You can even
implement it partially, if you want to provide only the table name or only the
column names to be used by [Rx].

If you use the commandline tool `rowx`, it will generate for you structures for
all tables in the database and these structs will implement SqlxMeta.
*/
type SqlxMeta[R Rowx] interface {
	Table() string
	Columns() []string
}

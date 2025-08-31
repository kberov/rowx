package rx

import "database/sql"

/*
Rowx is an empty interface and generic constraint for database records.
Any struct type implements it.
*/
type Rowx interface{}

/*
SqlxModel is an interface and generic constraint for working with a set of
database records. [Rx] fully implements SqlxModel. You can embed (extend)
Rx to get automatically its implementation and override some of its
methods.
*/
type SqlxModel[R Rowx] interface {
	SetData([]R) SqlxModel[R]
	SqlxInserter[R]
	SqlxSelector[R]
	SqlxUpdater[R]
	SqlxDeleter[R]
}

/*
SqlxInserter can be implemented to insert records in a table. It is fully
implemented by [Rx].
*/
type SqlxInserter[R Rowx] interface {
	Data() []R
	SqlxMeta[R]
	Insert() (sql.Result, error)
}

/*
SqlxUpdater can be implemented to update records in a table. It is fully
implemented by [Rx].
*/
type SqlxUpdater[R Rowx] interface {
	Data() []R
	Table() string
	Update([]string, string) (sql.Result, error)
}

/*
SqlxGetter can be implemented to get one record from the database. It is
fully implemented by [Rx].
*/
type SqlxGetter[R Rowx] interface {
	SqlxMeta[R]
	Get(string, ...any) (*R, error)
}

/*
SqlxSelector can be implemented to select records from a table or view. It
is fully implemented by [Rx].
*/
type SqlxSelector[R Rowx] interface {
	SqlxGetter[R]
	Select(string, any, ...int) ([]R, error)
}

/*
SqlxDeleter can be implemented to delete records from a table. It is
fully implemented by [Rx].
*/
type SqlxDeleter[R Rowx] interface {
	Table() string
	Delete(string, any) (sql.Result, error)
}

/*
SqlxMeta can be implemented to return the name of the table in the database for
the implementing type and the slice with its column names. It is fully
implemented by [Rx].

If you implement this interface, its methods will be called by [Rx] everywhere
where table name or a slice of columns are needed. You can even implement it
partially, if you want to provide only the table name or only the column names
to be used by [Rx].
*/
type SqlxMeta[R Rowx] interface {
	Table() string
	Columns() []string
}

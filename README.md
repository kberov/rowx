# Rowx
A commandline tool and generic data type to use with sqlx.
Package rx provides a minimalistic object to database-rows mapper by using the
scanning capabilities of [sqlx]. It is also an SQL builder using SQL templates.

At runtime the templates get filled in with metadata (tables' and columns'
names from the provided data structures) and WHERE clauses, written by you -
the programmer in SQL. The rendered by [fasttemplate] SQL query is prepared and
executed by [sqlx].

In other words, package `rx` provides functions, interfaces and a generic data
type [Rx], which wraps data structures, provided by you or generated from
existing tables by [Generate]. [Rx] implements the provided interfaces to
execute CRUD operations. The relations' constraints are left to be managed by
the database.

To ease the programmer's work with the database, `rx` provides two functions -
[Migrate] and [Generate]. The first executes sets of SQL statements from a file
to migrate the the database schema to a new state and the second re-generates
the structs, mappped to rows in tables.

By default the current implementation assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. You can mark such fields with tags. See below.

# Synopsis

```bash
	# Have an existing or newly created database. Generate a model package
	# from it using the built-in commandline tool `rowx`.
	cd to/your/project/root
	# make a directory for your package, named for example "model"
	mkdir -p internal/example/model
	# Generate structures from all tables in the database, implementing
	# SqlxMeta interface.
	rowx generate -dsn /some/path/test.sqlite -package ./internal/example/model
```
```go
	// Use the structures elsewhere in your code.
	// ...
	// Have a structure, mapping a table row, generated in
	// ./internal/example/model/model_tables.go.
	type Users struct {
		LoginName string
		// ...
		ID        int32
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
```
```bash
	// Create new migration file or add to an existing one a new set of SQL
	// statements to migrate the database to a new state.
	cd to/your/project/root
	vim data/migrations_01.sql
	// Migrate.
	./rowx migrate -sql_file data/migrations_01.sql -dsn=/tmp/test.sqlite -direction=up
	// Run generate again to reflect the changes in the schema.
	rowx generate -dsn /some/path/test.sqlite -package ./internal/example/model
```
Edit your code, which uses the structures, if needed.
...and so the life of the application continues further on.

[sqlx]: https://github.com/jmoiron/sqlx
[fasttemplate]: https://github.com/valyala/fasttemplate


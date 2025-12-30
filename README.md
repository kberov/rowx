# Rowx
Rowx provides a commandline tool `rowx` and  a generic data type `Rx` in
package `rx` to use with [sqlx].

Package [rx] provides a minimalistic object to database-rows mapper by using the
scanning capabilities of [sqlx] and provided by it interfaces. It is also an
SQL builder using SQL templates.

At runtime the templates get filled in with metadata (tables' and columns'
names from the provided data structures) and WHERE clauses, written by you -
the programmer in SQL. The rendered by [fasttemplate] SQL query is prepared and
executed by [sqlx].

In other words, package `rx` provides functions, interfaces and a generic data
type `rx.Rx`, which wraps data structures, provided by you or generated from
existing tables by `rx.Generate`. `rx.Rx` implements the provided interfaces to
execute CRUD operations. The relations' constraints are left to be managed by
the database.

To ease the programmer's work with the database, `rx` provides two functions -
`Migrate` and `Generate`. The first executes sets of SQL statements from a file
to migrate the the database schema to a new state and the second re-generates
the structs, mappped to rows in tables. Both functions can be used on the
commandline. See package `rx` for details.

[rx]: https://github.com/kberov/rowx@v0.83.0/rx
[sqlx]: https://github.com/jmoiron/sqlx
[fasttemplate]: https://github.com/valyala/fasttemplate


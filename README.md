# Rowx
A generic data type to use with sqlx.


Package rx provides a minimalistic object to database-rows mapper and wrapper for
[sqlx]. It provides interfaces and a generic data type, and implements the
provided interfaces to work easily with sets of database records. The
relations' constraints are left to be managed by the database and you. This may
be improved in a future release.

By default the current implementation assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. You can mark such fields with tags. See below.

# Synopsis

```go
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

	r, e := rx.NewRx(users).Insert()
	if e != nil {
		fmt.Fprintf(os.Stderr, "Got error from m.Insert(): %s", e.Error())
		return
	}
	//...
```

[sqlx]: https://github.com/jmoiron/sqlx

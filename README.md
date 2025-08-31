# Rowx
A generic data type to use with sqlx.

Package rx provides a minimalistic object to database-rows mapper and wrapper
for [sqlx]. It provides functions, interfaces and a generic data type.
It implements the provided interfaces to work easily with sets of database
records. The relations' constraints are left to be managed by the database and
you. This may be improved in a future release.

By default the current implementation assumes that the primary key name is
`ID`. Of course the primary key can be more than one column and with arbitrary
name. You can mark such fields with tags. See below.

# Synopsis
```
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
	//

// A custom type, which implements rx.SqlxMeta[U].
/*
   type U struct {
	table     string
	LoginName string
	ID        int32 `rx:"id,auto"`
   }
   func (u *U) Table() string {
	if u.table == "" {
	   	u.table = `users`
	}
	return u.table
   }
   func (u *U) Columns() []string {
	return []string{`id`, `login_name`}
   }
*/
m := rx.NewRx[U]()
u, e := m.Get(`id=:id`, U{ID: 1})
if e != nil {
	fmt.Println("Error:", e.Error())
}
fmt.Printf("ID: %d, LoginName: %s", u.ID, u.LoginName)
// Output:
// ID: 1, LoginName: first
```

[sqlx]: https://github.com/jmoiron/sqlx


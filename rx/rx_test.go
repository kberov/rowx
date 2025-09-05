package rx_test

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kberov/rowx/rx"
)

var schema = `
PRAGMA foreign_keys = OFF;
CREATE TABLE users (
id INTEGER PRIMARY KEY AUTOINCREMENT,
login_name varchar(100) UNIQUE,
group_id INTEGER DEFAULT NULL REFERENCES groups(id),
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);

INSERT INTO users(id,group_id,changed_by,login_name) VALUES (0,0,0,'superadmin');

CREATE TABLE groups (
id INTEGER PRIMARY KEY AUTOINCREMENT,
name VARCHAR(100) UNIQUE NOT NULL,
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);

INSERT INTO groups(id,name, changed_by) VALUES (0,'superadmin',0);
INSERT INTO groups(id,name, changed_by) VALUES (1,'admins',NULL);
INSERT INTO groups(id,name, changed_by) VALUES (2,'guests',NULL);
INSERT INTO groups(id,name, changed_by) VALUES (3,'editors',NULL);
INSERT INTO groups(id,name, changed_by) VALUES (4,'commenters',NULL);
CREATE TABLE user_group (
--  'ID of the user belonging to the group with group_id.'
  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
--  'ID of the group to which the user with user_id belongs.'
  group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  PRIMARY KEY(user_id, group_id)
);
CREATE TABLE foo(
	bar INTEGER PRIMARY KEY AUTOINCREMENT,
	description VARCHAR(255) NOT NULL DEFAULT '',
	id VARCHAR(56) UNIQUE NOT NULL DEFAULT ''
);
PRAGMA foreign_keys = ON;
`

type Users struct {
	LoginName string
	GroupID   sql.NullInt32
	ChangedBy sql.NullInt32
	ID        int32 `rx:"id,auto"`
}

var users = []Users{
	Users{LoginName: "first", ChangedBy: sql.NullInt32{0, false}},
	Users{LoginName: "the_second", ChangedBy: sql.NullInt32{1, true}},
	Users{LoginName: "the_third", ChangedBy: sql.NullInt32{1, true}},
}

type Groups struct {
	Name      string
	ChangedBy sql.NullInt32
	ID        int32 `rx:"id,auto"`
}

// Stollen from sqlx_test.go.
func multiExec(e sqlx.Execer, query string) {
	stmts := strings.Split(query, ";\n")
	if len(strings.Trim(stmts[len(stmts)-1], " \n\t\r")) == 0 {
		stmts = stmts[:len(stmts)-1]
	}
	for _, s := range stmts {
		_, err := e.Exec(s)
		if err != nil {
			fmt.Println(err, s)
		}
	}
}

func init() {
	// rx.DSN = ":memory:"
	// rx.DriverName = `sqlite3`
	multiExec(rx.DB(), schema)
}

type UserGroup struct {
	rx.Rx[UserGroup]
	UserID  int32
	GroupID int32
	// Used only as bind parameters during UPDATE and maybe other queries. Must
	// be a named struct, known at compile time!
	Where whereParams `rx:"where,-"` // - : Do not treat this field as column.
}
type whereParams struct{ GroupID int32 }

// Note: the order of test matters, because they modify the same data and each
// next test relies on the current state of the data.
// TODO: Someday, maybe, make the order of execution not important or use
// something like TestMain.

// A custom type, which implements rx.SqlxMeta.
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

var uColumns = []string{`id`, `login_name`}

func (u *U) Columns() []string {
	return uColumns
}

func TestImplementsSqlxMeta(t *testing.T) {
	reQ := require.New(t)
	m := rx.NewRx[U]()
	u, e := m.Get(`id=0`)
	reQ.NoError(e)
	reQ.Equal(`users`, u.Table())
	reQ.Equal(uColumns, u.Columns())
	t.Logf(`Expected User from database: %#v`, u)
	t.Logf(`Instantiated: %#v`, m)
}

func TestTryEmbed(t *testing.T) {
	reQ := require.New(t)
	ug := new(UserGroup)
	reQ.Equal(`user_group`, ug.Table())
	expectedCols := []string{`user_id`, `group_id`}
	slices.Sort(expectedCols)
	slices.Sort(ug.Columns())
	reQ.Equal(expectedCols, ug.Columns())
	// Insert some users (the usual way) to meet the foreign key constraint.
	rs, err := rx.NewRx[Users](users...).Insert()
	reQ.NoError(err)
	rows, errAff := rs.LastInsertId()
	reQ.NoError(errAff)
	reQ.Equal(int64(3), rows)
	ugDataIns := []UserGroup{
		UserGroup{UserID: 1, GroupID: 0},
		UserGroup{UserID: 1, GroupID: 1},
		UserGroup{UserID: 2, GroupID: 2},
		UserGroup{UserID: 3, GroupID: 3},
		UserGroup{UserID: 1, GroupID: 4},
		UserGroup{UserID: 2, GroupID: 4},
		UserGroup{UserID: 3, GroupID: 4},
	}
	ug.SetData(ugDataIns)
	rs, err = ug.Insert()
	reQ.NoError(err)
	rows, errAff = rs.LastInsertId()
	reQ.NoError(errAff)
	reQ.Equal(int64(7), rows)
	// Update some rows - move some user(3) to another group(2).
	ugDataUpd := []UserGroup{
		UserGroup{
			UserID: 3,
			// new (to be updated in the database) value: 2
			GroupID: 2, Where: whereParams{
				// existing in the database value: 4
				GroupID: 4,
			},
		},
	}
	ug.SetData(ugDataUpd)
	//							set columns										WHERE struct
	rs, err = ug.Update([]string{`group_id`}, `user_id=:user_id AND group_id=:where.group_id`)
	reQ.NoError(err)
	rows, errAff = rs.RowsAffected()
	reQ.NoError(errAff)
	reQ.Equal(int64(1), rows)
	// Get the row to see what we did.
	row, err := ug.Get(
		`user_id = :uid AND group_id = :gid`,
		map[string]any{`uid`: 3, `gid`: ug.Data()[0].Where.GroupID})
	if err != nil {
		t.Logf(`err: %s`, err.Error())
	}
	t.Logf("Get updated row: %d|%d", row.UserID, row.GroupID)
	// Delete the inserted users, so the next tests pass. "ON DELETE
	// CASCADE" will delete all the user_group rows. Also reset the sequence for
	// AUTOINCREMENT for table users, to allow the primary key to start from 1.
	rs, err = rx.NewRx[Users]().Delete(`id>=:id`, map[string]any{`id`: 0})
	reQ.NoError(err)
	rows, errAff = rs.RowsAffected()
	reQ.NoError(errAff)
	reQ.Equal(int64(4), rows)
	_ = rx.DB().MustExec(`UPDATE sqlite_sequence SET seq = 0 WHERE name = 'users'`)
	// ugData, e := ug.Select(`user_id>0`, nil)
	// t.Logf("See if there is something left in UserGroup:%+v; err: %+v", ugData, e)
}

func TestNewModelNoData(t *testing.T) {
	// For subsequent call to Select(...) or Delete(...)....
	// If no Rowx are passed, NewRx needs a type parameter to know
	// which type to instantiate.
	m := rx.NewRx[Users]()
	if m == nil {
		t.Error("Could not instantiate Rx")
	}
}

func TestNewModelWithData(t *testing.T) {
	// Type parameter is guessed from the type of the parameters.
	m := rx.NewRx(users...)
	expected := len(users)
	if i := len(m.Data()); i != expected {
		t.Errorf("Expected rows: %d. Got: %d!", expected, i)
	}
}

func TestTable(t *testing.T) {
	type AVeryLongAndComplexTableName struct {
		ID int32
	}
	m := &rx.Rx[AVeryLongAndComplexTableName]{}
	if table := m.Table(); table != `a_very_long_and_complex_table_name` {
		t.Errorf("wrong table '%s'", table)
	} else {
		t.Logf("Instantiated type: %#v\n TableName: %s\n", m, table)
	}
}

func TestCamelToSnake(t *testing.T) {
	slovo := map[string]string{
		"Кънигы":              "кънигы",
		"НашитеКънигы":        "нашите_кънигы",
		"OwnerID":             "owner_id",
		"PageType":            "page_type",
		"UsersInvoicesLastID": "users_invoices_last_id",
		"ID":                  "id",
		"ИД":                  "ид",
	}

	for k, v := range slovo {
		t.Run(k, func(t *testing.T) {
			slova := rx.CamelToSnake(k)
			t.Logf("%s => %s|%s", k, slova, v)
			if slova != v {
				t.Fail()
			}
		})
	}
}

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		name          string
		tableOrColumn string
		typeOrField   string
	}{
		{name: `long`, tableOrColumn: `a_very_long_and_complex_table_name`, typeOrField: `AVeryLongAndComplexTableName`},
		{name: `short`, tableOrColumn: `id`, typeOrField: `ID`},
		{name: `longUtf8`, tableOrColumn: `и_още_една_невъзможна_таблица`, typeOrField: `ИОщеЕднаНевъзможнаТаблица`},
		{name: `късутф8`, tableOrColumn: `ид`, typeOrField: `ИД`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equalf(t, tc.typeOrField, rx.SnakeToCamel(tc.tableOrColumn),
				`SnakeToCamel("%s") should return "%s"`, tc.tableOrColumn, tc.typeOrField)
			t.Logf(`SnakeToCamel("%s") returns "%s"`, tc.tableOrColumn, tc.typeOrField)
		})
	}
}

func TestColumns(t *testing.T) {
	tests := []struct {
		name string
		data []Users
	}{
		{
			name: `RxWithData`,
			data: users,
		},
		{
			name: `RxNoData`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := rx.NewRx[Users](tc.data...)
			if len(m.Columns()) == 0 {
				t.Errorf("Expected to have columns but we did not find any.")
			}
			t.Logf("columns are: %#v", m.Columns())
		})
	}
}

func TestSingleInsert(t *testing.T) {
	reQ := require.New(t)
	m := rx.NewRx[Users](users[0])

	r, e := m.Insert()
	reQ.NoErrorf(e, "Got error from m.Insert(): %v", e)

	id, e := r.LastInsertId()
	reQ.NoErrorf(e, "Error: %v", e)
	t.Logf("LastInsertId: %d", id)

	rows, e := r.RowsAffected()
	reQ.NoErrorf(e, "Error: %v", e)
	t.Logf("RowsAffected: %d", rows)

	u := &Users{}
	_ = rx.DB().Get(u, `SELECT * FROM users WHERE id=?`, 1)
	reQ.Equalf(users[0].LoginName, u.LoginName, "Expected LoginName: %s. Got: %s!", users[0].LoginName, u.LoginName)
	t.Logf(`First selected user: %#v`, u)
}

func TestMultyInsert(t *testing.T) {
	// t.Logf("Starting from second user: %#v;", users[1:])
	m := rx.NewRx(users[1:]...)
	r, e := m.Insert()
	require.NoErrorf(t, e, "sql.Result:%#v; Error:%#v;", r, e)
	t.Logf("sql.Result:%#v; Error:%#v;", r, e)
}

var testsForTestSelect = []struct {
	name, where   string
	errContains   string
	bindData      map[string]any
	lAndOff       []int
	lastID        int32
	expectedError bool
}{
	{
		// Does a SELECT with default LIMIT and OFFSET, without any WHERE clauses.
		name:     `All`,
		where:    ``,
		bindData: nil,
		lastID:   3,
	},
	{
		// Does a SELECT with LIMIT 2
		name:     `WithLimit`,
		where:    ``,
		bindData: nil,
		lAndOff:  []int{2, 0},
		lastID:   2,
	},
	{
		// Does a SELECT with LIMIT 2 and OFFSET 1
		name:     `WithLimitAndOffset`,
		where:    ``,
		bindData: nil,
		lAndOff:  []int{2, 1},
		lastID:   3,
	},
	{
		// Does a SELECT with WHERE id<:id
		name:     `WithWhere`,
		where:    `id >:id`,
		bindData: map[string]any{`id`: 1},
		lastID:   3,
	},
	{
		name:          `PrepareError`,
		where:         `WHERE `,
		bindData:      map[string]any{`id`: 1},
		lastID:        3,
		expectedError: true,
		errContains:   `syntax error`,
	},
	{
		name:          `SelectError`,
		where:         `id=:id`,
		bindData:      map[string]any{},
		lastID:        3,
		expectedError: true,
		errContains:   `could not find name id`,
	},
	{
		name:     `SelectIN`,
		where:    `id IN(:ids)`,
		bindData: map[string]any{`ids`: []int{1, 2, 3}},
		lastID:   3,
	},
	{
		name:     `SelectOrderByDesc`,
		where:    `id IN(:ids) ORDER BY id DESC`,
		bindData: map[string]any{`ids`: []int{1, 2, 3}},
		lastID:   1,
	},
}

func TestSelect(t *testing.T) {
	m := rx.NewRx[Users]()
	for _, tc := range testsForTestSelect {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := m.Select(tc.where, tc.bindData, tc.lAndOff...)
			if err != nil && !tc.expectedError {
				t.Errorf("Error: %#v", err)
				return
			} else if err != nil && tc.expectedError {
				t.Logf("Expected error: %#v", err)
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf(`Error does not contain expected string: '%s'`, tc.errContains)
				}
				return
			}
			dataLen := uint(len(rows))
			if rows[dataLen-1].ID != tc.lastID {
				t.Errorf("Expected last.ID to be %d. Got %d", tc.lastID, rows[dataLen-1].ID)
			}
		})
	}
}

var testsForTestUpdate = []struct {
	Rx          rx.SqlxModel[Users]
	name        string
	where       string
	selectWhere string
	selectBind  map[string]any
	columns     []string
	affected    int64
	dbError     bool
}{
	{
		name:        `One`,
		where:       `id=:id`,
		selectWhere: `id=:id`,
		Rx: rx.NewRx(Users{LoginName: `first_updated`, ID: 1,
			GroupID: sql.NullInt32{Valid: true, Int32: 0}}),
		affected:   1,
		columns:    []string{`Login_name`},
		selectBind: map[string]any{`id`: 1},
		dbError:    false,
	},
	{
		name: `ManyUniqueConstraintFail`,
		// this WHERE clause will produce UNIQUE CONSTRAINT Error, because login_name is UNIQUE.
		where:       `id IN(SELECT id FROM users WHERE ID>1)`,
		selectWhere: `id IN(SELECT id FROM users WHERE ID>1)`,
		Rx: rx.NewRx(
			Users{LoginName: `second_updated`, ID: 2},
			Users{LoginName: `third_updated`, ID: 3, GroupID: sql.NullInt32{Valid: true, Int32: 2}},
		),
		affected: 0,
		columns:  []string{`LoginName`, `group_id`},
		dbError:  true,
	},
	{
		name: `ManyUniqueConstraintOK`,
		// this WHERE clause will NOT produce UNIQUE CONSTRAINT Error, because id is PRIMARY KEY.
		where: `id = :id`,
		Rx: rx.NewRx(
			Users{LoginName: `second_updated_ok`, ID: 2, GroupID: sql.NullInt32{Valid: true, Int32: 2}},
			Users{LoginName: `third_updated_ok`, ID: 3, GroupID: sql.NullInt32{Valid: true, Int32: 3}},
		),
		affected:    2,
		columns:     []string{`login_name`, `GroupID`},
		dbError:     false,
		selectWhere: `id IN(:id)`,
		selectBind:  map[string]any{`id`: []any{2, 3}},
	},
}

func TestUpdate(t *testing.T) {
	for i, tc := range testsForTestUpdate {
		t.Run(tc.name, func(t *testing.T) {
			var (
				r sql.Result
				e error
			)

			r, e = tc.Rx.Update(tc.columns, tc.where)
			if e != nil && tc.dbError {
				t.Logf("Error updating records: '%#v' was expected.", e)
				return
			} else if e != nil && !tc.dbError {
				t.Errorf("Unexpected error: '%#v'!...", e)
				return
			}
			// Strange how RowsAffected is always 1 even when it is obvious
			// that two rows were affected.
			rows, _ := r.RowsAffected()
			t.Logf("*sql.Result.RowsAffected(): %d", rows)

			data, e := rx.NewRx[Users]().Select(tc.selectWhere, tc.selectBind)
			if e != nil {
				t.Errorf(`Error in m.Select: %#v`, e)
				return
			}
			if data[0].LoginName != tc.Rx.Data()[0].LoginName {
				t.Errorf(`Expected login_name to be %s, but it is %s!`,
					tc.Rx.Data()[0].LoginName, data[0].LoginName)
			}

			if i == 1 {
				groupID := tc.Rx.Data()[0].GroupID.Int32
				if groupID != data[0].GroupID.Int32 {
					t.Errorf("Expected group_id to be set to %#v! It was set to: %#v",
						groupID, data[0].GroupID.Int32)
				}
			}
			t.Logf("Updated records: %#v", data)
		})
	}
}

func TestDelete(t *testing.T) {
	// TODO: add test case for bind where bind is a struct.
	tests := []struct {
		bind        any
		name, where string
		affected    int64
	}{
		{
			name:     `One`,
			where:    `id=:some_id`,
			bind:     map[string]any{`some_id`: 1},
			affected: 1,
		},
		{
			name:     `Many`,
			where:    `id > 1`,
			affected: 2,
		},
	}
	m := rx.NewRx[Users]()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, e := m.Delete(tc.where, tc.bind)
			if e != nil {
				t.Errorf("Error deleting one record: %#v", e)
				return
			}
			if rows, e := r.RowsAffected(); e != nil {
				t.Errorf("Error: %v", e)
			} else if rows != tc.affected {
				t.Errorf("Expected rows to be affected were %d. Got %d", tc.affected, rows)
			} else {
				t.Logf("RowsAffected: %d", rows)
			}
		})
	}
}

type myModel[R rx.Rowx] struct {
	rx.Rx[R]
	data []R
}

func (m *myModel[R]) Data() []R {
	return m.data
}

func (m *myModel[R]) mySelect() ([]R, error) {
	rx.Logger.Debugf(`executing SELECT from an extending type: %T`, m)
	err := rx.DB().Select(&m.data, `SELECT * from groups limit 100`)
	return m.data, err
}

func TestWrap(t *testing.T) {
	reQ := require.New(t)
	// ---
	mm := &myModel[Groups]{}
	reQ.Equalf(`groups`, mm.Table(), `Wrong table for myModel: %s`, mm.Table())

	data, err := mm.Select(`id >:id`, rx.SQLMap{`id`: 1})
	reQ.NoError(err, `Unexpected error:%#v`, err)
	reQ.Equalf(3, len(data), `Expected 3 rows from the database but got %d.`, len(data))

	m := &myModel[Groups]{}
	data, _ = m.mySelect()
	reQ.Equalf(5, len(data), `Expected 5 rows from the database but got %d.`, len(data))
	reQ.Equalf(data[0], m.Data()[0], `m.Data() and data should point to the same data!`)

	// test behaviour of tag option `auto`
	type Foo struct {
		Description string
		ID          string `id:"id,no_auto"`
		Foo         uint32 `rx:"bar,auto"`
	}

	foo := rx.NewRx[Foo](
		Foo{Description: `first record`},
		Foo{Description: `second record`},
	)
	for i, f := range foo.Data() {
		f.ID = fmt.Sprintf("%x", sha256.Sum224([]byte(f.Description)))
		foo.Data()[i] = f
	}
	_, err = foo.Insert()
	reQ.NoError(err)
	// Using the keyword WHERE is optional, but can be written even if only for
	// expressiveness.
	firstFoo, err := foo.Get(`WHERE bar=1`)
	reQ.NoError(err)
	d, e := rx.NewRx[Foo]().Select(`id IN(:ids)`, map[string]any{`ids`: []int32{1, 2}})
	t.Logf("%+v, %v", d, e)
	reQ.Equal(`first record`, firstFoo.Description)
	secondFoo, err := foo.Get(`bar=2`)
	reQ.NoError(err)
	reQ.Equal(`second record`, secondFoo.Description)
}

func TestPanics(t *testing.T) {
	tests := []struct {
		fn   func()
		name string
	}{
		{
			name: `InsertNoData`,
			fn: func() {
				g := rx.NewRx[Groups]()
				_, _ = g.Insert()
			},
		},
		{
			name: `UpdateNoData`,
			fn: func() {
				g := rx.NewRx[Groups]()
				_, _ = g.Update(g.Columns(), `1`)
			},
		},
		{
			name: `RenderSQLTemplate NoTemplateFound`,
			fn: func() {
				rx.RenderSQLTemplate(`NOSUCH`, map[string]any{})
			},
		},
		{
			name: `TypeToSnakeCase`,
			fn: func() {
				r := new(struct{ ID int16 })
				rx.TypeToSnake(r)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectPanic(t, tc.fn)
		})
	}
}

func expectPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MISSING PANIC")
		} else {
			t.Log(r)
		}
	}()
	f()
}

func TestResetDB(t *testing.T) {
	drops := `
PRAGMA foreign_keys = OFF;
DROP TABLE users;
DROP TABLE user_group;
DROP TABLE groups;
DROP TABLE foo;
`
	multiExec(rx.DB(), drops)
	multiExec(rx.DB(), schema)
	t.Log(`Database is reset.`)
}

var aStr = `           WHERE bar=1`

func Benchmark_stringContainsWhere(b *testing.B) {
	for b.Loop() {
		strings.Contains(aStr, strings.TrimPrefix(strings.ToLower(aStr), ` `))
	}
}

// ...but matching with regexp is much more reliable than checking if the string
// just contains where.
var containsWhere = regexp.MustCompile(`(?i:^\s*where\s)`)

func Benchmark_regexpMatchWhere(b *testing.B) {
	for b.Loop() {
		containsWhere.MatchString(aStr)
	}
}

func Fuzz_containsWhere(f *testing.F) {
	for _, v := range []string{aStr, `where i=1`, `    Where e>0`, `wheRe.Int32 `} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, in string) {
		t.Logf(`in:%v`, in)
		if !containsWhere.MatchString(in) {
			if strings.Contains(aStr, strings.ToLower(`where`)) {
				t.Errorf(`Expected to match '%s', but it did not!`, in)
			}
		}
	})
}

func ExampleNewRx() {
	// If no Rowx are passed, NewRx needs a type parameter to know
	// which type to instantiate for subsequent call to Select(...) or Delete(...)....
	m := rx.NewRx[Users]()
	fmt.Printf(" %#T\n", m)
	// Output:
	// *rx.Rx[github.com/kberov/rowx/rx_test.Users]
	//
}

func ExampleNewRx_with_param() {
	// To Inser(...)  Update(...) []Users in the database, no type parameter is
	// needed.
	m := rx.NewRx(users...)
	last := m.Data()[len(m.Data())-1]
	fmt.Printf("Last user: %s", last.LoginName)
	// Output:
	// Last user: the_third
}

func ExampleRx_Data() {
	type Users struct {
		LoginName string
		GroupID   sql.NullInt32
		ChangedBy sql.NullInt32
		ID        int32 `rx:"id,auto"`
	}
	// []Users to be inserted (or updated, (LoginName is UNIQUE)).
	var users = []Users{
		Users{LoginName: "first", ChangedBy: sql.NullInt32{1, true}},
		Users{LoginName: "the_second", ChangedBy: sql.NullInt32{1, true}},
	}
	// Type parameter is guessed from the type of the parameters.
	m := rx.NewRx(users...)
	for _, u := range m.Data() {
		fmt.Printf("User.LoginName: %s, User.ChangedBy.Int32: %d\n", u.LoginName, u.ChangedBy.Int32)
	}
	// Output:
	// User.LoginName: first, User.ChangedBy.Int32: 1
	// User.LoginName: the_second, User.ChangedBy.Int32: 1
}

func ExampleRx_SetData() {
	ugDataIns := []UserGroup{
		UserGroup{UserID: 1, GroupID: 1},
		UserGroup{UserID: 2, GroupID: 2},
		UserGroup{UserID: 3, GroupID: 3},
		UserGroup{UserID: 1, GroupID: 4},
		UserGroup{UserID: 2, GroupID: 4},
	}
	ug := rx.NewRx[UserGroup]().SetData(ugDataIns)
	for i, row := range ug.Data() {
		fmt.Printf("%d: UserID: %d; GroupID: %d\n", i+1, row.UserID, row.GroupID)
	}
	// Output:
	//
	// 1: UserID: 1; GroupID: 1
	// 2: UserID: 2; GroupID: 2
	// 3: UserID: 3; GroupID: 3
	// 4: UserID: 1; GroupID: 4
	// 5: UserID: 2; GroupID: 4
}

func ExampleRx_Table() {
	type WishYouWereHere struct {
		SongName string
		ID       uint32
	}
	f := WishYouWereHere{SongName: `Shine On You Crazy Diamond`}
	fmt.Printf("TableName: %s\n", rx.NewRx(f).Table())

	// Output:
	// TableName: wish_you_were_here
	//
}

func ExampleRx_Columns() {
	type Books struct {
		Title  string
		Author string
		Body   string
		ID     uint32
		//...
	}

	b := Books{Title: `Нова земя`, Author: `Иванъ Вазовъ`, Body: `По стръмната южна урва на Амбарица...`}
	columns := rx.NewRx(b).Columns()
	fmt.Printf("Columns: %+v\n", columns)

	// Output:
	// Columns: [title author body id]
}

func ExampleRx_Insert() {
	_, e := rx.NewRx(users...).Insert()
	if e != nil {
		println(`Error inserting new users:`, e)
	}
	// udata, e := rx.NewRx[Users]().Select(`id>=0`, nil)
	// fmt.Printf("Selected []Users %+v; %+v\n", udata, e)
	groupRs, e := rx.NewRx[Groups](Groups{Name: `fifth`}).Insert()
	if e != nil {
		println(`Error inserting new group:`, e)
	}
	lastGroupID, _ := groupRs.LastInsertId()
	fmt.Printf("Inserted new group with id: %d\n", lastGroupID)

	usrs := []Users{
		Users{LoginName: `fourth`, GroupID: sql.NullInt32{Int32: 4, Valid: true}},
		Users{LoginName: `fifth`, GroupID: sql.NullInt32{Int32: 5, Valid: true}},
	}
	r, err := rx.NewRx(usrs...).Insert()

	if err == nil {
		last, _ := r.LastInsertId()
		fmt.Println(`Last inserted user id:`, last)
		// Output:
		// Inserted new group with id: 5
		// Last inserted user id: 5
		return
	}
	fmt.Printf("err: %s", err)
}

func ExampleRx_Get() {
	// A long time ago in a galaxy far, far away....
	// m := rx.NewRx(users...)
	// ...
	// r, e := m.Insert()
	// fmt.Printf("sql.Result:%#v; Error:%#v;", r, e)
	// ...
	// d, e := rx.NewRx[Users]().Select(`id>0`, nil)
	// fmt.Printf("%+v; e:%+v", d, e)
	// ...
	// Now
	bindVars := struct{ ID int32 }{ID: 4}
	u, err := rx.NewRx[Users]().Get(`id=:id`, bindVars)
	if err == nil {
		fmt.Println(u.LoginName)
		// Output:
		// fourth
		return
	}
	fmt.Printf("err: %s\n", err)
}

func ExampleRx_Select() {
	bind := struct{ IDs []uint }{IDs: []uint{4, 5}}
	u := rx.NewRx[Users]()
	data, err := u.Select(`id IN(:ids) ORDER BY id DESC`, bind)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(`Last two records in descending order:`)
	for _, u := range data {
		fmt.Printf("%d: %s\n", u.ID, u.LoginName)
	}

	// We can reuse the *Rx object for this parameter type for many and
	// different SQL queries.
	fmt.Println("\nUp to DefaultLimit records with OFFSET 0 in the default order:")
	data, err = u.Select(``, nil)
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, u := range data {
		fmt.Printf("%d: %s\n", u.ID, u.LoginName)
	}
	// Output:
	// Last two records in descending order:
	// 5: fifth
	// 4: fourth
	//
	// Up to DefaultLimit records with OFFSET 0 in the default order:
	// 0: superadmin
	// 1: first
	// 2: the_second
	// 3: the_third
	// 4: fourth
	// 5: fifth
}

func ExampleRx_Update() {
	type whereBind struct{ GroupID uint32 }
	type UserGroup struct {
		rx.Rx[UserGroup]
		UserID  uint32
		GroupID uint32
		// Used only as bind parameters during UPDATE and maybe in other
		// queries. Must be a named struct, known at compile time!
		Where whereBind `rx:"where,-"` // - : Do not treat this field as column.
	}
	// rx.Rx can be embedded and used from within your record structure or
	// specialized type.
	ug := new(UserGroup)
	ugData := []UserGroup{
		UserGroup{UserID: 4, GroupID: 4},
		UserGroup{UserID: 5, GroupID: 5},
	}
	ug.SetData(ugData)
	_, e := ug.Insert()
	if e != nil {
		fmt.Println("Error inserting into user_group:", e.Error())
	}

	// Update one or many rows - move some user(5) to another group(4).
	ugDataUpd := []UserGroup{
		UserGroup{
			UserID: 5,
			// new value (to be updated in the database). Current value: 5
			GroupID: 4,
			Where: whereBind{
				// existing in the database value: 5
				GroupID: 5,
			},
		},
	}
	ug.SetData(ugDataUpd)
	//                    columns to be set                             the Where.GroupID field
	rs, err := ug.Update([]string{`group_id`}, `user_id=:user_id AND group_id=:where.group_id`)
	if err != nil {
		fmt.Println(err.Error())
	}
	affected, _ := rs.RowsAffected()
	fmt.Printf("RowsAffected: %d; err: %+v", affected, err)

	// Output:
	// RowsAffected: 1; err: <nil>
}

func ExampleSqlxMeta() {
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
}

/*
 */

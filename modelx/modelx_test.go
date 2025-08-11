package modelx_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kberov/rowx/modelx"
	_ "github.com/mattn/go-sqlite3"
)

var schema = `
PRAGMA foreign_keys = OFF;
CREATE TABLE users (
id INTEGER PRIMARY KEY AUTOINCREMENT,
login_name varchar(100) UNIQUE,
group_id INTEGER DEFAULT NULL REFERENCES groups(id),
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);
CREATE TABLE groups (
id INTEGER PRIMARY KEY AUTOINCREMENT,
name VARCHAR(100) UNIQUE NOT NULL,
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);
INSERT INTO groups(id,name, changed_by) VALUES (0,'superadmin',1);
INSERT INTO groups(id,name, changed_by) VALUES (1,'admins',1);
INSERT INTO groups(id,name, changed_by) VALUES (2,'editors',1);
INSERT INTO groups(id,name, changed_by) VALUES (3,'guests',1);
INSERT INTO groups(id,name, changed_by) VALUES (4,'commenters',1);
PRAGMA foreign_keys = ON;

`

type Users struct {
	ID        int32
	LoginName string
	GroupID   sql.NullInt32
	ChangedBy sql.NullInt32
}

var users = []Users{
	Users{LoginName: "first", ChangedBy: sql.NullInt32{1, true}},
	Users{LoginName: "the_second", ChangedBy: sql.NullInt32{1, true}},
	Users{LoginName: "the_third", ChangedBy: sql.NullInt32{1, true}},
}

type Groups struct {
	ID        int32
	Name      string
	ChangedBy sql.NullInt32
}

// Stollen from sqlx_test.go
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
	modelx.DSN = ":memory:"
	modelx.DriverName = `sqlite3`
	multiExec(modelx.DB(), schema)
}

func TestNewNoData(t *testing.T) {
	m := modelx.NewModel[Users]()
	if m == nil {
		t.Error("Could not instantiate Modelx")
	}
}

func TestNewWithData(t *testing.T) {
	// type parameter is guessed from the type of the parameters.
	m := modelx.NewModel(users...)
	expected := len(users)
	if i := len(m.Data()); i != expected {
		t.Errorf("Expected rows: %d. Got: %d!", expected, i)
	}
}

func TestTable(t *testing.T) {
	type AVeryLongAndComplexTableName struct {
	}
	m := &modelx.Modelx[AVeryLongAndComplexTableName]{}
	if table := m.Table(); table != `a_very_long_and_complex_table_name` {
		t.Errorf("wrong table '%s'", table)
	} else {
		t.Logf("Instantited type: %#v\n TableName: %s\n", m, table)
	}

}

func TestColumns(t *testing.T) {
	tests := []struct {
		name string
		data []Users
	}{
		{
			name: `WithData`,
			data: users,
		},
		{
			name: `NoData`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := modelx.NewModel[Users](tc.data...)
			if len(m.Columns()) == 0 {
				t.Errorf("Expected to have columns but we did not find any.")
			}
			t.Logf("columns are: %#v", m.Columns())
		})
	}
}

func TestSingleInsert(t *testing.T) {
	m := modelx.NewModel[Users](users[0])
	r, e := m.Insert()
	if e != nil {
		t.Errorf("Got error from m.Insert(): %v", e)
		return
	}
	if id, e := r.LastInsertId(); e != nil {
		t.Errorf("Error: %v", e)
	} else {
		t.Logf("LastInsertId: %d", id)
	}
	if r, e := r.RowsAffected(); e != nil {
		t.Errorf("Error: %v", e)
	} else {
		t.Logf("RowsAffected: %d", r)
	}
	u := &Users{}
	modelx.DB().Get(u, `SELECT * FROM users WHERE id=? LIMIT 1`, 1)
	if u.LoginName != users[0].LoginName {
		t.Errorf("Expected LoginName: %s. Got: %s!", users[0].LoginName, u.LoginName)
	}
	t.Logf(`First selected user: %#v`, u)
}

func TestMultyInsert(t *testing.T) {
	// t.Logf("Starting from second user: %#v;", users[1:])
	m := modelx.NewModel(users[1:]...)
	r, e := m.Insert()
	if e != nil {
		t.Errorf("sql.Result:%#v; Error:%#v;", r, e)
	}
	t.Logf("sql.Result:%#v; Error:%#v;", r, e)
}

func TestSelect(t *testing.T) {
	m := modelx.NewModel[Users]()
	tests := []struct {
		name, where   string
		bindData      map[string]any
		lAndOff       []int
		lastID        int32
		expectedError bool
		errContains   string
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
			where:    `WHERE id >:id`,
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
			where:         `WHERE id=:id`,
			bindData:      map[string]any{},
			lastID:        3,
			expectedError: true,
			errContains:   `could not find name id`,
		},
		{
			name:     `SelectIN`,
			where:    `WHERE id IN(:ids)`,
			bindData: map[string]any{`ids`: []int{1, 2, 3}},
			lastID:   3,
		},
		{
			name:     `SelectOrderByDesc`,
			where:    `WHERE id IN(:ids) ORDER BY id DESC`,
			bindData: map[string]any{`ids`: []int{1, 2, 3}},
			lastID:   1,
		},
	}
	for _, tc := range tests {
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
			dataLen := int32(len(rows))
			if rows[dataLen-1].ID != tc.lastID {
				t.Errorf("Expected last.ID to be %d. Got %d", tc.lastID, rows[dataLen-1].ID)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name, where, selectWhere string
		Modelx                   modelx.SqlxModel[Users]
		affected                 int64
		columns                  []string
		selectBind               map[string]any
		dbError                  bool
	}{
		{
			name:        `One`,
			where:       `WHERE id=:id`,
			selectWhere: `WHERE id=:id`,
			Modelx: modelx.NewModel(Users{LoginName: `first_updated`, ID: 1,
				GroupID: sql.NullInt32{Valid: true, Int32: 0}}),
			affected:   1,
			columns:    []string{`Login_name`},
			selectBind: map[string]any{`id`: 1},
			dbError:    false,
		},
		{
			name: `ManyUniqueConstraintFail`,
			// this WHERE clause will produce UNIQUE CONSTRAINT Error, because login_name is UNIQUE.
			where:       `WHERE id IN(SELECT id FROM users WHERE ID>1)`,
			selectWhere: `WHERE id IN(SELECT id FROM users WHERE ID>1)`,
			Modelx: modelx.NewModel(
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
			where: `WHERE id = :id`,
			Modelx: modelx.NewModel(
				Users{LoginName: `second_updated_ok`, ID: 2, GroupID: sql.NullInt32{Valid: true, Int32: 2}},
				Users{LoginName: `third_updated_ok`, ID: 3, GroupID: sql.NullInt32{Valid: true, Int32: 3}},
			),
			affected:    2,
			columns:     []string{`login_name`, `GroupID`},
			dbError:     false,
			selectWhere: `WHERE id IN(:id)`,
			selectBind:  map[string]any{`id`: []any{2, 3}},
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				r sql.Result
				e error
			)

			r, e = tc.Modelx.Update(tc.columns, tc.where)
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

			data, e := modelx.NewModel[Users]().Select(tc.selectWhere, tc.selectBind)
			if e != nil {
				t.Errorf(`Error in m.Select: %#v`, e)
				return
			}
			if data[0].LoginName != tc.Modelx.Data()[0].LoginName {
				t.Errorf(`Expected login_name to be %s, but it is %s!`,
					tc.Modelx.Data()[0].LoginName, data[0].LoginName)
			}

			if i == 1 {
				groupID := tc.Modelx.Data()[0].GroupID.Int32
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
	m := modelx.NewModel[Users]()
	// TODO: add test case for bind where bind is a struct.
	tests := []struct {
		name, where string
		bind        any
		affected    int64
	}{
		{
			name:     `One`,
			where:    `WHERE id=:some_id`,
			bind:     map[string]any{`some_id`: 1},
			affected: 1,
		},
		{
			name:     `Many`,
			where:    `WHERE id > 1`,
			affected: 2,
		},
	}
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

type myModel[R Groups] struct {
	modelx.Modelx[R]
	data []R
}

func (m *myModel[R]) Data() []R {
	return m.data
}
func (m *myModel[R]) mySelect() ([]R, error) {
	modelx.Logger.Debugf(`executing SELECT from an extending type: %T`, m)
	err := modelx.DB().Select(&m.data, `SELECT * from groups limit 100`)
	return m.data, err
}
func TestEmbed(t *testing.T) {
	// ---
	mm := &myModel[Groups]{}
	if mm.Table() != `groups` {
		t.Errorf(`Wrong table for myModel: %s`, mm.Table())
	}
	mm = new(myModel[Groups])
	if mm.Table() != `groups` {
		t.Errorf(`Wrong table for myModel: %s`, mm.Table())
		return
	}
	data, err := mm.Select(`WHERE id >:id`, modelx.SQLMap{`id`: 1})
	if err != nil {
		t.Errorf(`Unexpected error:%#v`, err)
	}
	if len(data) != 3 {
		t.Errorf(`Expected 3 rows from the database but got %d.`, len(data))
	}
	m := &myModel[Groups]{}
	data, _ = m.mySelect()

	if len(data) != 5 {
		t.Errorf(`Expected 5 rows from the database but got %d.`, len(data))
	}
	if data[0] != m.Data()[0] {
		t.Error(`m.Data() and data should point to the same data!`)
	}
	t.Logf("Extending object's m.Data(): %#v", m.Data())
}

func TestPanics(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: `InsertNoData`,
			fn: func() {
				g := modelx.NewModel[Groups]()
				g.Insert()
			},
		},
		{
			name: `NoTable`,
			fn: func() {
				modelx.NewModel[struct{ ID int16 }]().Table()
			},
		},
		{
			name: `RenderSQLTemplate NoTemplateFound`,
			fn: func() {
				modelx.RenderSQLTemplate(`NOSUCH`, map[string]any{})
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

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
	multiExec(modelx.DB(), schema)
}

func TestTable(t *testing.T) {
	m := &modelx.Modelx[Users]{}
	if table := m.Table(); table != "users" {
		t.Errorf("wrong table '%s'", table)
	} else {
		t.Logf("Instantited type: %#v\n TableName: %s\n", m, table)
		t.Logf("Modelx.Data: %#v\n", m.Data())
	}
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

func TestColumnsWithData(t *testing.T) {
	m := modelx.NewModel[Users](users...)
	if len(m.Columns()) == 0 {
		t.Errorf("Expected to have columns but we did not find any.")
	}
	t.Logf("columns are: %#v", m.Columns())
}

func TestColumnsNoData(t *testing.T) {
	m := modelx.NewModel[Users]()
	if len(m.Columns()) == 0 {
		t.Errorf("Expected to have columns but we did not find any.")
	}
	t.Logf("columns are: %#v", m.Columns())
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := m.Select(tc.where, tc.bindData, tc.lAndOff...)
			if err != nil && !tc.expectedError {
				t.Errorf("Error: %#v", err)
			} else if tc.expectedError && strings.Contains(err.Error(), tc.errContains) {
				t.Logf("Expected error: %#v", err)
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
		name, where           string
		Modelx                modelx.SqlxModel[Users]
		affected              int64
		excludeColumnsInWhere bool
		selectBind            map[string]any
		dbError               bool
	}{
		{
			name:  `OneExcludeID`,
			where: `WHERE id=:id`,
			Modelx: modelx.NewModel(Users{LoginName: `first_updated`, ID: 1,
				GroupID: sql.NullInt32{Valid: true, Int32: 0}}),
			affected:              1,
			excludeColumnsInWhere: true,
			selectBind:            map[string]any{`id`: 1},
			dbError:               false,
		},
		{
			name: `Many`,
			// this WHERE clause will produce UNIQUE CONSTRAINT Error
			where: `WHERE id IN(SELECT id FROM users WHERE ID>1)`,
			Modelx: modelx.NewModel(
				Users{LoginName: `second_updated`, ID: 2},
				Users{LoginName: `third_updated`, ID: 3, GroupID: sql.NullInt32{Valid: true, Int32: 2}},
			),
			affected:              2,
			excludeColumnsInWhere: true,
			dbError:               true,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				r sql.Result
				e error
			)

			r, e = tc.Modelx.Update(tc.where, tc.excludeColumnsInWhere)
			if e != nil && tc.dbError {
				t.Logf("Error updating records: '%#v' was expected.", e)
				return
			} else if e != nil && !tc.dbError {
				t.Errorf("Unexpected error: '%#v'!...", e)
				return
			}
			t.Logf("r: %#v", r)
			if rows, e := r.RowsAffected(); e != nil {
				t.Errorf("Error: %v", e)
			} else if rows != tc.affected {
				t.Errorf("Expected rows to be affected were %d. Got %d", tc.affected, rows)
			} else {
				t.Logf("RowsAffected: %d", rows)
			}

			data, e := modelx.NewModel[Users]().Select(tc.where, tc.selectBind)
			if e != nil {
				t.Errorf(`Error in m.Select: %#v`, e)
				return
			}
			if i == 0 && data[0].LoginName != tc.Modelx.Data()[0].LoginName {
				t.Errorf(`Expected login_name to be %s, but it is %s!`,
					tc.Modelx.Data()[0].LoginName, data[0].LoginName)
			}

			if i == 0 {
				groupID := tc.Modelx.Data()[0].GroupID
				if groupID != data[0].GroupID {
					t.Errorf("Expected group_id to be set to %#v! It was set to: %#v",
						groupID, data[0].GroupID)
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

func TestEmbed(t *testing.T) {
	type myModel[R modelx.SqlxRows] struct {
		modelx.Modelx[R]
	}
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

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
INSERT INTO groups(id,name, changed_by) VALUES (1,'admins',1);
PRAGMA foreign_keys = ON;

`
var users = []Users{
	Users{LoginName: "first", ChangedBy: sql.NullInt32{1, true}},
	Users{LoginName: "the_second", ChangedBy: sql.NullInt32{1, true}},
	Users{LoginName: "the_third", ChangedBy: sql.NullInt32{1, true}},
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

type Users struct {
	ID        int32
	LoginName string
	GroupID   sql.NullInt32
	ChangedBy sql.NullInt32
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
	m := modelx.NewModel[Users](users...)
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
	m := modelx.NewModel[Users](users[1:]...)
	r, e := m.Insert()
	t.Logf("sql.Result:%#v; Error:%#v;", r, e)
}

func TestSelect(t *testing.T) {
	m := modelx.NewModel[Users]()
	tests := []struct {
		name, where string
		bindData    map[string]any
		lAndOff     [2]int
		lastId      int32
	}{
		{
			// Does a SELECT with default LIMIT and OFFSET, without any WHERE clauses.
			name:     `All`,
			where:    ``,
			bindData: nil,
			lastId:   3,
		},
		{
			// Does a SELECT with LIMIT 2
			name:     `WithLimit`,
			where:    ``,
			bindData: nil,
			lAndOff:  [...]int{2, 0},
			lastId:   2,
		},
		{
			// Does a SELECT with LIMIT 2 and OFFSET 1
			name:     `WithLimitAndOffset`,
			where:    ``,
			bindData: nil,
			lAndOff:  [...]int{2, 1},
			lastId:   3,
		},
		{
			// Does a SELECT with WHERE id<:id
			name:     `WithWhere`,
			where:    `WHERE id >:id`,
			bindData: map[string]any{`id`: 1},
			lastId:   3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := m.Select(tc.where, tc.bindData, tc.lAndOff)
			if err != nil {
				t.Errorf("Error: %#v", err)
			}
			dataLen := int32(len(m.Data()))
			if m.Data()[dataLen-1].ID != tc.lastId {
				t.Errorf("Expected last.ID to be %d. Got %d", tc.lastId, m.Data()[dataLen-1].ID)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	m := modelx.NewModel[Users]()
	tests := []struct {
		name, where string
		set         map[string]any
		bind        map[string]any
		affected    int64
	}{
		{
			name:     `One`,
			set:      map[string]any{`login_name`: `first_updated`},
			where:    `WHERE id=:id`,
			bind:     map[string]any{`id`: 1},
			affected: 1,
		},
		{
			name:     `ManyNoBind`,
			set:      map[string]any{`group_id`: 1},
			where:    `WHERE id IN(SELECT id FROM users WHERE ID>1)`,
			affected: 2,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, e := m.Update(tc.set, tc.where, tc.bind)
			if e != nil {
				t.Errorf("Error updating one record: %#v", e)
				return
			}
			if rows, e := r.RowsAffected(); e != nil {
				t.Errorf("Error: %v", e)
			} else if rows != tc.affected {
				t.Errorf("Expected rows to be affected were %d. Got %d", tc.affected, rows)
			} else {
				t.Logf("RowsAffected: %d", rows)
			}

			m.Select(tc.where, tc.bind, [2]int{0, 0})
			if i == 0 && m.Data()[0].LoginName != tc.set[`login_name`] {
				t.Errorf(`Expected login_name to be %s, but it is %s!`,
					tc.set[`login_name`], m.Data()[0].LoginName)
			}
			if i == 1 {
				for _, v := range m.Data() {
					group_id := tc.set["group_id"]
					if group_id != int(v.GroupID.Int32) {
						t.Errorf("expected group_id to be set to %d! It is: %d",
							group_id, v.GroupID.Int32)
					}
				}
			}
			t.Logf("Updated records: %#v", m.Data())
		})
	}
}

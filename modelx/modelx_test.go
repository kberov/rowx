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
	Users{ID: 1, LoginName: "first", ChangedBy: sql.NullInt32{1, true}},
	Users{ID: 2, LoginName: "the_second", ChangedBy: sql.NullInt32{1, true}},
	Users{ID: 3, LoginName: "the_third", ChangedBy: sql.NullInt32{1, true}},
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
	r, _ := m.Insert()
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

func TestSimplestSelect(t *testing.T) {

	m := modelx.NewModel[Users]()
	err := m.Select("", nil, [2]int{0, 0})
	if err != nil {
		t.Errorf("Error: %#v", err)
	}
	t.Logf("Returned Data: %#v", m.Data())
}

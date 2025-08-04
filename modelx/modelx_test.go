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
CREATE TABLE users (
id INTEGER PRIMARY KEY AUTOINCREMENT,
login_name varchar(100) UNIQUE,
group_id INTEGER DEFAULT NULL REFERENCES groups(id),
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);
CREATE TABLE groups (
id INTEGER PRIMARY KEY AUTOINCREMENT,
name VARCHAR(100) UNIQUE NOT NULL,
changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT);
`

func MultiExec(e sqlx.Execer, query string) {
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
	MultiExec(modelx.DB(), schema)
}

func TestTable(t *testing.T) {
	m := &modelx.Modelx[Users]{}
	if table := m.TableName(); table != "users" {
		t.Fatal("wrong table", table)
	} else {
		t.Logf("Instantited type: %#v\n TableName: %s\n", m, table)
		t.Logf("Modelx.Data: %#v\n", m.Data)
	}
}

func TestNewNoData(t *testing.T) {
	m := modelx.NewModel[Users]()
	if m == nil {
		t.Fatal("Could not instantiate Modelx")
	}
}

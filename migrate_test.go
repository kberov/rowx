package migrate_test

import (
	"testing"

	"github.com/kberov/rowx/rx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestMigrate_up(t *testing.T) {
	reQ := require.New(t)
	dsn := rx.DSN // `rx/testdata/migrate_test.sqlite`
	err := rx.Migrate(`testdata/migr.sql`, dsn, `up`)
	reQ.ErrorContains(err, `no such file or directory`)

	err = rx.Migrate(`rx/testdata/migrations_01.sql`, dsn, `up`)
	reQ.NoErrorf(err, `Unexpected error during migration: %v`, err)
	// now all migrations, found in migrations_01 must be registered as applied
	// in rx.MigrationsTable
	rxM := rx.NewRx[rx.Migrations]()
	appliedMigrations, err := rxM.Select(`direction=:dir`, rx.SQLMap{`dir`: `up`})
	reQ.NoErrorf(err, `Unexpected error during Select: %v`, err)
	reQ.Equal(2, len(appliedMigrations))
}

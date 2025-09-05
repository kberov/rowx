package migrate_test

import (
	"testing"

	"github.com/kberov/rowx/rx"
	"github.com/stretchr/testify/require"
)

func TestMigrate(t *testing.T) {
	reQ := require.New(t)
	err := rx.Migrate(`testdata/migrations.sql`, rx.DSN, `up`)
	reQ.NoError(err)
}

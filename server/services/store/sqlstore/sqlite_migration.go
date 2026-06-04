//go:build !(linux && loong64)

package sqlstore

import (
	"database/sql"

	drivers "github.com/mattermost/morph/drivers"
	sqlite "github.com/mattermost/morph/drivers/sqlite"
)

func newSQLiteMigrationDriver(db *sql.DB) (drivers.Driver, error) {
	return sqlite.WithInstance(db)
}

//go:build linux && loong64

package sqlstore

import (
	"database/sql"
	"errors"

	drivers "github.com/mattermost/morph/drivers"
)

func newSQLiteMigrationDriver(_ *sql.DB) (drivers.Driver, error) {
	return nil, errors.New("sqlite migrations are not supported on linux/loong64")
}

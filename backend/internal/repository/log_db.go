package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/lib/pq"
)

const (
	logDBMaxOpenConns    = 1
	logDBMaxIdleConns    = 1
	logDBApplicationName = "sub2api_log_writer"
)

// LogDB owns the small database pool reserved for asynchronous operational
// and audit log writes. Keeping it separate prevents best-effort logging from
// consuming or disrupting the pool used by authentication and billing.
type LogDB struct {
	db *sql.DB
}

func ProvideLogDB(cfg *config.Config) (*LogDB, error) {
	if cfg == nil {
		return nil, errors.New("nil config for log database")
	}

	connector, err := pq.NewConnector(logDatabaseDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("build log database connector: %w", err)
	}

	db := sql.OpenDB(connector)
	settings := clampDBPoolSettings(cfg)
	db.SetMaxOpenConns(logDBMaxOpenConns)
	db.SetMaxIdleConns(logDBMaxIdleConns)
	db.SetConnMaxLifetime(settings.ConnMaxLifetime)
	db.SetConnMaxIdleTime(settings.ConnMaxIdleTime)

	return &LogDB{db: db}, nil
}

func logDatabaseDSN(cfg *config.Config) string {
	return cfg.Database.DSNWithTimezone(cfg.Timezone) + " application_name=" + logDBApplicationName
}

func (l *LogDB) SQLDB() *sql.DB {
	if l == nil {
		return nil
	}
	return l.db
}

func (l *LogDB) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProvideLogDBUsesIsolatedSingleConnectionPool(t *testing.T) {
	cfg := &config.Config{
		Timezone: "UTC",
		Database: config.DatabaseConfig{
			Host:                   "127.0.0.1",
			Port:                   5432,
			User:                   "postgres",
			DBName:                 "sub2api",
			SSLMode:                "disable",
			Schema:                 "sub2api",
			MaxOpenConns:           8,
			MaxIdleConns:           2,
			ConnMaxLifetimeMinutes: 30,
			ConnMaxIdleTimeMinutes: 5,
		},
	}

	logDB, err := ProvideLogDB(cfg)
	require.NoError(t, err)
	require.NotNil(t, logDB.SQLDB())
	require.Equal(t, logDBMaxOpenConns, logDB.SQLDB().Stats().MaxOpenConnections)
	dsn := logDatabaseDSN(cfg)
	require.True(t, strings.Contains(dsn, "application_name="+logDBApplicationName))
	require.True(t, strings.Contains(dsn, "search_path=sub2api,public"))
	require.NoError(t, logDB.Close())
}

func TestProvideLogDBRejectsNilConfig(t *testing.T) {
	logDB, err := ProvideLogDB(nil)
	require.Nil(t, logDB)
	require.EqualError(t, err, "nil config for log database")
}

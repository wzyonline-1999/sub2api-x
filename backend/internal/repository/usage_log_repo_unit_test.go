//go:build unit

package repository

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSafeDateFormat(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		expected    string
	}{
		// 合法值
		{"hour", "hour", "YYYY-MM-DD HH24:00"},
		{"day", "day", "YYYY-MM-DD"},
		{"week", "week", "IYYY-IW"},
		{"month", "month", "YYYY-MM"},

		// 非法值回退到默认
		{"空字符串", "", "YYYY-MM-DD"},
		{"未知粒度 year", "year", "YYYY-MM-DD"},
		{"未知粒度 minute", "minute", "YYYY-MM-DD"},

		// 恶意字符串
		{"SQL 注入尝试", "'; DROP TABLE users; --", "YYYY-MM-DD"},
		{"带引号", "day'", "YYYY-MM-DD"},
		{"带括号", "day)", "YYYY-MM-DD"},
		{"Unicode", "日", "YYYY-MM-DD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeDateFormat(tc.granularity)
			require.Equal(t, tc.expected, got, "safeDateFormat(%q)", tc.granularity)
		})
	}
}

func TestBuildUsageLogBatchInsertQuery_UsesConflictDoNothing(t *testing.T) {
	log := &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-batch-no-update",
		Model:        "gpt-5",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.2,
		ActualCost:   1.2,
		CreatedAt:    time.Now().UTC(),
	}
	prepared := prepareUsageLogInsert(log)

	query, _ := buildUsageLogBatchInsertQuery([]string{usageLogBatchKey(log.RequestID, log.APIKeyID)}, map[string]usageLogInsertPrepared{
		usageLogBatchKey(log.RequestID, log.APIKeyID): prepared,
	})

	require.Contains(t, query, "ON CONFLICT (request_id, api_key_id) DO NOTHING")
	require.NotContains(t, strings.ToUpper(query), "DO UPDATE")
}

func TestPrepareUsageLogInsert_IncludesTruncatedSessionMetadata(t *testing.T) {
	source := "header_session_id"
	hash := "0123456789abcdef"
	explicit := true
	rawSessionID := strings.Repeat("s", 1100)

	log := &service.UsageLog{
		UserID:          1,
		APIKeyID:        2,
		AccountID:       3,
		RequestID:       "req-session-metadata",
		Model:           "gpt-5",
		SessionID:       &rawSessionID,
		SessionIDSource: &source,
		SessionHash:     &hash,
		SessionExplicit: &explicit,
		CreatedAt:       time.Now().UTC(),
	}

	prepared := prepareUsageLogInsert(log)
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	sessionID, ok := prepared.args[len(prepared.args)-5].(sql.NullString)
	require.True(t, ok)
	require.True(t, sessionID.Valid)
	require.Equal(t, strings.Repeat("s", service.UsageLogSessionIDMaxLength), sessionID.String)

	sessionSource, ok := prepared.args[len(prepared.args)-4].(sql.NullString)
	require.True(t, ok)
	require.True(t, sessionSource.Valid)
	require.Equal(t, source, sessionSource.String)

	sessionHash, ok := prepared.args[len(prepared.args)-3].(sql.NullString)
	require.True(t, ok)
	require.True(t, sessionHash.Valid)
	require.Equal(t, hash, sessionHash.String)

	sessionExplicit, ok := prepared.args[len(prepared.args)-2].(sql.NullBool)
	require.True(t, ok)
	require.True(t, sessionExplicit.Valid)
	require.Equal(t, explicit, sessionExplicit.Bool)
}

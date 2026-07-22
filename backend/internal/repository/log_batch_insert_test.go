package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const (
	expectedOpsSystemLogsBatchInsert = `INSERT INTO ops_system_logs (created_at, host, level, component, message, request_id,
client_request_id, user_id, api_key_id, account_id, platform, model, extra) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13),($14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`
	expectedAuditLogsBatchInsert = `INSERT INTO audit_logs (created_at, actor_user_id, actor_email, actor_role, auth_method,
credential_masked, action, method, path, request_id, client_ip, user_agent,
request_body, status_code, latency_ms, extra) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16),($17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32)`
)

func TestBuildMultiRowInsertQuery(t *testing.T) {
	query, args := buildMultiRowInsertQuery("events", "id, name", [][]any{
		{int64(1), "first"},
		{int64(2), "second"},
	})

	require.Equal(t, "INSERT INTO events (id, name) VALUES ($1,$2),($3,$4)", query)
	require.Equal(t, []any{int64(1), "first", int64(2), "second"}, args)
}

func TestBatchInsertSystemLogsUsesOneNormalizedMultiRowInsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 7, 22, 1, 2, 3, 0, time.FixedZone("UTC+8", 8*60*60))
	userID := int64(7)
	zeroID := int64(0)
	inputs := []*service.OpsInsertSystemLogInput{
		nil,
		{CreatedAt: createdAt, Host: " node-1 ", Level: " WARN ", Message: " request finished ", RequestID: " req-1 ", UserID: &userID, AccountID: &zeroID, Platform: " openai ", Model: " gpt-5.6-sol "},
		{CreatedAt: createdAt, Level: " ", Message: "missing level"},
		{CreatedAt: createdAt, Level: "info", Message: "  "},
		{CreatedAt: createdAt, Level: " ERROR ", Component: " gateway ", Message: " upstream failed ", ExtraJSON: ` {"status":503} `},
	}

	mock.ExpectExec(expectedOpsSystemLogsBatchInsert).
		WithArgs(
			createdAt.UTC(), "node-1", "warn", "app", "request finished", "req-1", nil,
			userID, nil, nil, "openai", "gpt-5.6-sol", "{}",
			createdAt.UTC(), nil, "error", "gateway", "upstream failed", nil, nil,
			nil, nil, nil, nil, nil, `{"status":503}`,
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	inserted, err := (&opsRepository{db: db}).BatchInsertSystemLogs(context.Background(), inputs)
	require.NoError(t, err)
	require.Equal(t, int64(2), inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchInsertSystemLogsReturnsNoInsertedRowsOnStatementFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	writeErr := errors.New("system log insert failed")
	createdAt := time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC)
	mock.ExpectExec(`INSERT INTO ops_system_logs (created_at, host, level, component, message, request_id,
client_request_id, user_id, api_key_id, account_id, platform, model, extra) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`).
		WithArgs(createdAt, nil, "info", "app", "ok", nil, nil, nil, nil, nil, nil, nil, "{}").
		WillReturnError(writeErr)

	inserted, err := (&opsRepository{db: db}).BatchInsertSystemLogs(context.Background(), []*service.OpsInsertSystemLogInput{
		{CreatedAt: createdAt, Level: "info", Message: "ok"},
	})
	require.ErrorIs(t, err, writeErr)
	require.Zero(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchInsertSystemLogsSkipsInvalidRowsWithoutQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	inserted, err := (&opsRepository{db: db}).BatchInsertSystemLogs(context.Background(), []*service.OpsInsertSystemLogInput{
		nil,
		{Level: "", Message: "missing level"},
		{Level: "info", Message: ""},
	})
	require.NoError(t, err)
	require.Zero(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogBatchInsertUsesOneNormalizedMultiRowInsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 7, 22, 9, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	actorID := int64(9)
	logs := []*service.AuditLog{
		nil,
		{
			CreatedAt: createdAt, ActorUserID: &actorID, ActorEmail: " admin@example.com ", ActorRole: " admin ",
			AuthMethod: " jwt ", CredentialMasked: " sk-*** ", Action: " user.update ", Method: " PUT ",
			Path: " /api/v1/admin/users/7 ", RequestID: " req-audit-1 ", ClientIP: " 192.0.2.1 ",
			UserAgent: " test-agent ", RequestBody: `{"role":"user"}`, StatusCode: 200, LatencyMs: 17,
			Extra: map[string]any{"source": "admin"},
		},
		{CreatedAt: createdAt, Action: "auth.login", StatusCode: 401},
	}

	mock.ExpectExec(expectedAuditLogsBatchInsert).
		WithArgs(
			createdAt.UTC(), actorID, "admin@example.com", "admin", "jwt", "sk-***", "user.update", "PUT",
			"/api/v1/admin/users/7", "req-audit-1", "192.0.2.1", "test-agent", `{"role":"user"}`, 200, int64(17), `{"source":"admin"}`,
			createdAt.UTC(), nil, "", "", "", "", "auth.login", "", "", "", "", "", "", 401, int64(0), "{}",
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	inserted, err := (&auditLogRepository{db: db}).BatchInsert(context.Background(), logs)
	require.NoError(t, err)
	require.Equal(t, int64(2), inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogBatchInsertReturnsNoInsertedRowsOnStatementFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	writeErr := errors.New("audit log insert failed")
	createdAt := time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC)
	mock.ExpectExec(`INSERT INTO audit_logs (created_at, actor_user_id, actor_email, actor_role, auth_method,
credential_masked, action, method, path, request_id, client_ip, user_agent,
request_body, status_code, latency_ms, extra) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`).
		WithArgs(createdAt, nil, "", "", "", "", "auth.login", "", "", "", "", "", "", 200, int64(0), "{}").
		WillReturnError(writeErr)

	inserted, err := (&auditLogRepository{db: db}).BatchInsert(context.Background(), []*service.AuditLog{
		{CreatedAt: createdAt, Action: "auth.login", StatusCode: 200},
	})
	require.ErrorIs(t, err, writeErr)
	require.Zero(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

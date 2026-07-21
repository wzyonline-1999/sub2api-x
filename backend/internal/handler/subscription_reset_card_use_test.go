package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// SQLite lacks SELECT ... FOR UPDATE. This adapter leaves every real Ent
// query/update/transaction in place and strips only the lock suffix before
// SQLite executes it.
type resetCardUseSQLiteLockCompatDriver struct {
	dialect.Driver
}

func (d *resetCardUseSQLiteLockCompatDriver) Dialect() string { return dialect.Postgres }

func (d *resetCardUseSQLiteLockCompatDriver) Query(ctx context.Context, query string, args, v any) error {
	return d.Driver.Query(ctx, stripResetCardUseSQLiteLock(query), args, v)
}

func (d *resetCardUseSQLiteLockCompatDriver) Exec(ctx context.Context, query string, args, v any) error {
	return d.Driver.Exec(ctx, stripResetCardUseSQLiteLock(query), args, v)
}

func (d *resetCardUseSQLiteLockCompatDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return &resetCardUseSQLiteLockCompatTx{Tx: tx}, nil
}

type resetCardUseSQLiteLockCompatTx struct {
	dialect.Tx
}

func (tx *resetCardUseSQLiteLockCompatTx) Query(ctx context.Context, query string, args, v any) error {
	return tx.Tx.Query(ctx, stripResetCardUseSQLiteLock(query), args, v)
}

func (tx *resetCardUseSQLiteLockCompatTx) Exec(ctx context.Context, query string, args, v any) error {
	return tx.Tx.Exec(ctx, stripResetCardUseSQLiteLock(query), args, v)
}

func stripResetCardUseSQLiteLock(query string) string {
	query = strings.ReplaceAll(query, " FOR UPDATE SKIP LOCKED", "")
	return strings.ReplaceAll(query, " FOR UPDATE", "")
}

type failOnceUserMarkSucceededRepo struct {
	*userMemoryIdempotencyRepoStub
	failNext bool
}

func (r *failOnceUserMarkSucceededRepo) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	if r.failNext {
		r.failNext = false
		return errors.New("mark succeeded failed")
	}
	return r.userMemoryIdempotencyRepoStub.MarkSucceeded(ctx, id, responseStatus, responseBody, expiresAt)
}

type resetCardUseHandlerFixture struct {
	ctx            context.Context
	router         *gin.Engine
	client         *dbent.Client
	userID         int64
	subscriptionID int64
	grantID        int64
}

func newResetCardUseHandlerFixture(t *testing.T) *resetCardUseHandlerFixture {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:reset_card_use_handler_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	sqliteDriver := entsql.OpenDB(dialect.SQLite, db)
	client := dbent.NewClient(dbent.Driver(sqliteDriver))
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))
	t.Cleanup(func() { _ = client.Close() })

	user := client.User.Create().
		SetEmail(fmt.Sprintf("reset-card-handler-%d@example.com", time.Now().UnixNano())).
		SetPasswordHash("hash").
		SaveX(ctx)
	group := client.Group.Create().
		SetName(fmt.Sprintf("Reset Card Handler %d", time.Now().UnixNano())).
		SetSubscriptionType(domain.SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).
		SaveX(ctx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	subscription := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-24 * time.Hour)).
		SetExpiresAt(now.Add(29 * 24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetDailyWindowStart(now.Add(-4 * time.Hour)).
		SetDailyUsageUsd(4.5).
		SetDailyWindowVersion(7).
		SaveX(ctx)
	grant := client.SubscriptionResetCardGrant.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetIssuedCount(2).
		SetRemainingCount(2).
		SetStatus("active").
		SaveX(ctx)

	compatClient := dbent.NewClient(dbent.Driver(&resetCardUseSQLiteLockCompatDriver{Driver: sqliteDriver}))
	subscriptionService := service.NewSubscriptionService(nil, repository.NewUserSubscriptionRepository(client), nil, compatClient, nil)
	t.Cleanup(subscriptionService.Stop)
	handler := NewSubscriptionHandler(subscriptionService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(withUserSubject(user.ID))
	router.POST("/api/v1/subscriptions/:id/reset-card/use", handler.UseResetCard)

	return &resetCardUseHandlerFixture{
		ctx:            ctx,
		router:         router,
		client:         client,
		userID:         user.ID,
		subscriptionID: subscription.ID,
		grantID:        grant.ID,
	}
}

func (f *resetCardUseHandlerFixture) use(t *testing.T, key string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/subscriptions/%d/reset-card/use", f.subscriptionID), nil)
	request.Header.Set("Idempotency-Key", key)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func TestUseResetCardRecoversAfterMarkSucceededFailureAndConsumesOnce(t *testing.T) {
	fixture := newResetCardUseHandlerFixture(t)
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })
	idempotencyRepo := &failOnceUserMarkSucceededRepo{
		userMemoryIdempotencyRepoStub: newUserMemoryIdempotencyRepoStub(),
		failNext:                      true,
	}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(idempotencyRepo, service.DefaultIdempotencyConfig()))

	first := fixture.use(t, "manual-use-ambiguous")
	second := fixture.use(t, "manual-use-ambiguous")

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "true", first.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Recovered"))
	require.Contains(t, first.Body.String(), `"usage"`)
	require.Contains(t, first.Body.String(), `"subscription"`)
	require.Equal(t, 1, fixture.client.SubscriptionResetCardUsage.Query().CountX(fixture.ctx))
	require.Equal(t, 1, fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, fixture.grantID).RemainingCount)
	loadedSubscription := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscriptionID)
	require.Zero(t, loadedSubscription.DailyUsageUsd)
	require.Equal(t, int64(8), loadedSubscription.DailyWindowVersion)
}

func TestUseResetCardStoreUnavailableBeforeExecutionDoesNotConsume(t *testing.T) {
	fixture := newResetCardUseHandlerFixture(t)
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(userStoreUnavailableRepoStub{}, service.DefaultIdempotencyConfig()))

	responseRecorder := fixture.use(t, "manual-use-preflight-store-down")

	require.Equal(t, http.StatusServiceUnavailable, responseRecorder.Code)
	require.Empty(t, responseRecorder.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, 0, fixture.client.SubscriptionResetCardUsage.Query().CountX(fixture.ctx))
	require.Equal(t, 2, fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, fixture.grantID).RemainingCount)
	loadedSubscription := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscriptionID)
	require.Equal(t, 4.5, loadedSubscription.DailyUsageUsd)
	require.Equal(t, int64(7), loadedSubscription.DailyWindowVersion)
}

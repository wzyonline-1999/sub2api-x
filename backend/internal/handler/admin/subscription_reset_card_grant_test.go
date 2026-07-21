//go:build unit

package admin

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type resetCardGrantHandlerFixture struct {
	router  *gin.Engine
	client  *dbent.Client
	service *service.SubscriptionService
	userID  int64
	groupID int64
	adminID int64
}

func newResetCardGrantHandlerFixture(t *testing.T) *resetCardGrantHandlerFixture {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:reset_card_grant_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	user := client.User.Create().
		SetEmail("reset-card-user@example.com").
		SetPasswordHash("hash").
		SaveX(ctx)
	adminUser := client.User.Create().
		SetEmail("reset-card-admin@example.com").
		SetPasswordHash("hash").
		SetRole(domain.RoleAdmin).
		SaveX(ctx)
	group := client.Group.Create().
		SetName("Reset Card Plan").
		SetSubscriptionType(domain.SubscriptionTypeSubscription).
		SaveX(ctx)

	subscriptionService := service.NewSubscriptionService(repository.NewGroupRepository(client, db), nil, nil, client, nil)
	t.Cleanup(subscriptionService.Stop)
	handler := NewSubscriptionHandler(subscriptionService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: adminUser.ID})
		c.Next()
	})
	router.POST("/api/v1/admin/subscription-reset-cards/grants", handler.GrantResetCards)

	return &resetCardGrantHandlerFixture{
		router:  router,
		client:  client,
		service: subscriptionService,
		userID:  user.ID,
		groupID: group.ID,
		adminID: adminUser.ID,
	}
}

func (f *resetCardGrantHandlerFixture) request(t *testing.T, count int, key string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"user_id":%d,"group_id":%d,"count":%d,"notes":"customer care"}`, f.userID, f.groupID, count))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscription-reset-cards/grants", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func TestGrantResetCardsRecoversAfterMarkSucceededFailureWithoutDuplicateGrant(t *testing.T) {
	fixture := newResetCardGrantHandlerFixture(t)
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })
	idempotencyRepo := &failOnceMarkSucceededRepo{
		memoryIdempotencyRepoStub: newMemoryIdempotencyRepoStub(),
		failNext:                  true,
	}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(idempotencyRepo, service.DefaultIdempotencyConfig()))

	first := fixture.request(t, 3, "grant-reset-cards-ambiguous")
	second := fixture.request(t, 3, "grant-reset-cards-ambiguous")

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "true", first.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Recovered"))
	require.Contains(t, first.Body.String(), `"remaining_count":3`)
	require.Contains(t, first.Body.String(), `"user_email":"reset-card-user@example.com"`)
	require.Contains(t, first.Body.String(), `"group_name":"Reset Card Plan"`)
	require.Equal(t, 1, fixture.client.SubscriptionResetCardGrant.Query().CountX(context.Background()))
}

func TestGrantSubscriptionResetCardsBusinessRequestIDRecoversAndRejectsPayloadReuse(t *testing.T) {
	fixture := newResetCardGrantHandlerFixture(t)
	input := service.GrantSubscriptionResetCardsInput{
		UserID:         fixture.userID,
		GroupID:        fixture.groupID,
		Count:          2,
		IssuedBy:       fixture.adminID,
		Notes:          "customer care",
		IdempotencyKey: "grant-reset-cards-business-key",
	}

	first, err := fixture.service.GrantSubscriptionResetCards(context.Background(), input)
	require.NoError(t, err)
	retry, err := fixture.service.GrantSubscriptionResetCards(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, first.ID, retry.ID)
	require.Equal(t, 1, fixture.client.SubscriptionResetCardGrant.Query().CountX(context.Background()))

	input.Count = 4
	conflicting, err := fixture.service.GrantSubscriptionResetCards(context.Background(), input)
	require.Nil(t, conflicting)
	require.Equal(t, infraerrors.Reason(service.ErrIdempotencyKeyConflict), infraerrors.Reason(err))
	require.Equal(t, 1, fixture.client.SubscriptionResetCardGrant.Query().CountX(context.Background()))
}

func TestGrantSubscriptionResetCardsRequiresIdempotencyKey(t *testing.T) {
	fixture := newResetCardGrantHandlerFixture(t)

	first := fixture.request(t, 3, "")
	second := fixture.request(t, 3, "")

	require.Equal(t, http.StatusBadRequest, first.Code)
	require.Equal(t, http.StatusBadRequest, second.Code)
	require.Contains(t, first.Body.String(), "IDEMPOTENCY_KEY_REQUIRED")
	require.Zero(t, fixture.client.SubscriptionResetCardGrant.Query().CountX(context.Background()))
}

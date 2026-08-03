package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type databasePingerFunc func(context.Context) error

func (f databasePingerFunc) PingContext(ctx context.Context) error {
	return f(ctx)
}

func commonRouteResponse(t *testing.T, database DatabasePinger, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	RegisterCommonRoutes(r, database)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestHealthIsLivenessOnly(t *testing.T) {
	called := false
	recorder := commonRouteResponse(t, databasePingerFunc(func(context.Context) error {
		called = true
		return errors.New("database unavailable")
	}), "/health")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"ok"}`, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.False(t, called)
}

func TestReadyChecksDatabase(t *testing.T) {
	recorder := commonRouteResponse(t, databasePingerFunc(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.False(t, deadline.IsZero())
		return nil
	}), "/ready")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"ready","checks":{"database":"ok"}}`, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
}

func TestReadyFailsClosedWithoutLeakingDatabaseError(t *testing.T) {
	const sensitiveError = "password authentication failed for secret-role"
	recorder := commonRouteResponse(t, databasePingerFunc(func(context.Context) error {
		return errors.New(sensitiveError)
	}), "/ready")

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"status":"not_ready","checks":{"database":"unavailable"}}`, recorder.Body.String())
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "password")
	require.NotContains(t, recorder.Body.String(), "secret-role")
}

func TestReadyFailsClosedWithoutDatabaseDependency(t *testing.T) {
	recorder := commonRouteResponse(t, nil, "/ready")

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"status":"not_ready","checks":{"database":"unavailable"}}`, recorder.Body.String())
}

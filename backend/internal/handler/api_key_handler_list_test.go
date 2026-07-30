package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type apiKeyListRepositoryCapture struct {
	service.APIKeyRepository
	calls   int
	filters service.APIKeyListFilters
}

func (r *apiKeyListRepositoryCapture) ListByUserID(
	_ context.Context,
	userID int64,
	params pagination.PaginationParams,
	filters service.APIKeyListFilters,
) ([]service.APIKey, *pagination.PaginationResult, error) {
	r.calls++
	r.filters = filters
	return []service.APIKey{{
			ID:     1,
			UserID: userID,
			Key:    "sk-handler-list-test",
			Name:   "test",
			Status: service.StatusActive,
		}}, &pagination.PaginationResult{
			Total:    1,
			Page:     params.Page,
			PageSize: params.PageSize,
			Pages:    1,
		}, nil
}

func newAPIKeyListHandlerRouter(repo *apiKeyListRepositoryCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	apiKeyHandler := NewAPIKeyHandler(apiKeyService)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/keys", apiKeyHandler.List)
	return router
}

func TestAPIKeyListCanSkipLastUsedIP(t *testing.T) {
	repo := &apiKeyListRepositoryCapture{}
	router := newAPIKeyListHandlerRouter(repo)
	request := httptest.NewRequest(http.MethodGet, "/keys?include_last_used_ip=false", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, repo.calls)
	require.True(t, repo.filters.SkipLastUsedIP)
}

func TestAPIKeyListIncludesLastUsedIPByDefault(t *testing.T) {
	repo := &apiKeyListRepositoryCapture{}
	router := newAPIKeyListHandlerRouter(repo)
	request := httptest.NewRequest(http.MethodGet, "/keys", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, repo.calls)
	require.False(t, repo.filters.SkipLastUsedIP)
}

func TestAPIKeyListCanExplicitlyIncludeLastUsedIP(t *testing.T) {
	repo := &apiKeyListRepositoryCapture{}
	router := newAPIKeyListHandlerRouter(repo)
	request := httptest.NewRequest(http.MethodGet, "/keys?include_last_used_ip=true", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, repo.calls)
	require.False(t, repo.filters.SkipLastUsedIP)
}

func TestAPIKeyListRejectsInvalidLastUsedIPFlag(t *testing.T) {
	repo := &apiKeyListRepositoryCapture{}
	router := newAPIKeyListHandlerRouter(repo)
	request := httptest.NewRequest(http.MethodGet, "/keys?include_last_used_ip=sometimes", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, repo.calls)
}

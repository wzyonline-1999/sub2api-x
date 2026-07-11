package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type visibleCapacityProviderStub struct {
	results      *service.VisibleCapacitySnapshot
	err          error
	requestedIDs []int64
}

func (s *visibleCapacityProviderStub) GetVisibleGroupCapacity(_ context.Context, userID int64) (*service.VisibleCapacitySnapshot, error) {
	s.requestedIDs = append(s.requestedIDs, userID)
	return s.results, s.err
}

func TestCapacityHandlerUnauthenticatedReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &visibleCapacityProviderStub{}
	h := &CapacityHandler{capacityService: provider}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/capacity/visible", nil)

	h.GetVisible(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, provider.requestedIDs)
}

func TestCapacityHandlerResponseFieldWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collectedAt := time.Date(2026, 7, 10, 8, 30, 0, 0, time.UTC)
	provider := &visibleCapacityProviderStub{results: &service.VisibleCapacitySnapshot{
		CollectedAt: collectedAt,
		Groups: []service.VisibleGroupCapacity{{
			GroupID:  10,
			Name:     "Claude",
			Platform: service.PlatformAnthropic,
			Concurrency: service.VisibleGroupConcurrency{
				Current:        3,
				Max:            8,
				Remaining:      5,
				LoadPercentage: 37.5,
				Waiting:        2,
			},
			AccountConcurrency: &service.VisibleAccountConcurrency{
				Current:            3,
				Max:                8,
				LoadPercentage:     37.5,
				ConfiguredAccounts: 2,
			},
			LoadCapacity: service.VisibleLoadCapacity{
				Available:  3,
				Total:      4,
				Percentage: 75,
			},
			QuotaLoad: service.VisibleQuotaLoad{
				FiveHour: &service.VisibleQuotaWindowLoad{
					LoadPercentage:   62.5,
					AccountsWithData: 3,
					TotalAccounts:    4,
				},
				SevenDay: &service.VisibleQuotaWindowLoad{
					LoadPercentage:   40,
					AccountsWithData: 2,
					TotalAccounts:    4,
				},
			},
		}},
	}}
	h := &CapacityHandler{capacityService: provider}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/capacity/visible", nil)
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})

	h.GetVisible(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int64{42}, provider.requestedIDs)

	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assertJSONKeys(t, envelope.Data, "collected_at", "groups")

	var groups []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope.Data["groups"], &groups))
	require.Len(t, groups, 1)
	assertJSONKeys(t, groups[0],
		"group_id", "name", "platform", "concurrency", "account_concurrency", "load_capacity", "quota_load")

	var groupConcurrency map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(groups[0]["concurrency"], &groupConcurrency))
	assertJSONKeys(t, groupConcurrency, "current", "max", "remaining", "load_percentage", "waiting")

	var accountConcurrency map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(groups[0]["account_concurrency"], &accountConcurrency))
	assertJSONKeys(t, accountConcurrency, "current", "max", "load_percentage", "configured_accounts")

	var loadCapacity map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(groups[0]["load_capacity"], &loadCapacity))
	assertJSONKeys(t, loadCapacity, "available", "total", "percentage")

	var quotaLoad map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(groups[0]["quota_load"], &quotaLoad))
	assertJSONKeys(t, quotaLoad, "five_hour", "seven_day")
	for _, windowKey := range []string{"five_hour", "seven_day"} {
		var window map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(quotaLoad[windowKey], &window))
		assertJSONKeys(t, window, "load_percentage", "accounts_with_data", "total_accounts")
	}

	for _, forbidden := range []string{
		"account_id", "account_name", "email", "credentials", "proxy", "error_message",
		"model_routing", "rate_multiplier", "is_exclusive", "subscription_type",
	} {
		require.NotContains(t, groups[0], forbidden, "public capacity row must not expose %q", forbidden)
	}
}

func assertJSONKeys(t *testing.T, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	require.Len(t, got, len(want))
	for _, key := range want {
		require.Contains(t, got, key)
	}
}

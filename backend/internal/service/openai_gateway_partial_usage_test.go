package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayForwardPreservesOnlyObservedPartialUsage(t *testing.T) {
	failedWithUsage := `data: {"type":"response.failed","response":{"id":"resp_partial","error":{"message":"upstream interrupted"},"usage":{"input_tokens":11,"output_tokens":4,"total_tokens":15}}}` + "\n\n"
	preOutputWithoutUsage := `data: {"type":"response.created","response":{"id":"resp_empty","status":"in_progress"}}` + "\n\n"

	tests := []struct {
		name        string
		passthrough bool
		upstream    string
		wantResult  bool
	}{
		{name: "responses observed tokens", upstream: failedWithUsage, wantResult: true},
		{name: "responses pre-output zero usage", upstream: preOutputWithoutUsage},
		{name: "passthrough observed tokens", passthrough: true, upstream: failedWithUsage, wantResult: true},
		{name: "passthrough pre-output zero usage", passthrough: true, upstream: preOutputWithoutUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runOpenAIPartialUsageForward(t, tt.passthrough, tt.upstream)

			require.Error(t, err)
			if !tt.wantResult {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
		})
	}
}

func TestHasObservedOpenAIUsageResultIncludesPerCallMediaAndSearch(t *testing.T) {
	require.False(t, HasObservedOpenAIUsageResult(nil))
	require.False(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{}))
	require.True(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{ImageCount: 1}))
	require.True(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{VideoCount: 1}))
	require.True(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{WebSearchCalls: 1}))
	require.True(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{SearchCount: 1}))
	require.True(t, HasObservedOpenAIUsageResult(&OpenAIForwardResult{AudioUsage: &AudioUsage{Mode: "tts", DurationOrUnits: 1}}))
}

func TestOpenAICompatPartialUsagePreservesBillingMetadataOnError(t *testing.T) {
	failedWithUsage := `data: {"type":"response.failed","response":{"id":"resp_partial","object":"response","model":"gpt-5.5","status":"failed","output":[],"error":{"code":"upstream_error","message":"input exceeds the context window"},"usage":{"input_tokens":11,"output_tokens":4,"total_tokens":15}}}` + "\n\n"

	tests := []struct {
		name string
		path string
		body []byte
		run  func(*OpenAIGatewayService, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":true,"service_tier":"priority","reasoning_effort":"high"}`),
			run: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			},
		},
		{
			name: "messages",
			path: "/v1/messages",
			body: []byte(`{"model":"gpt-5.5","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true,"output_config":{"effort":"high"}}`),
			run: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				c.Request.Header.Set("anthropic-beta", "fast-mode-2026-02-01")
				return svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"X-Request-Id": []string{"req_partial_compat_usage"},
				},
				Body: io.NopCloser(strings.NewReader(failedWithUsage)),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			account := &Account{
				ID:          9902,
				Name:        "partial-compat-usage",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Concurrency: 1,
				Credentials: map[string]any{
					"access_token":       "oauth-token",
					"chatgpt_account_id": "chatgpt-account",
				},
			}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := tt.run(svc, c, account, tt.body)

			require.Error(t, err)
			require.NotNil(t, result)
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.NotNil(t, result.ServiceTier)
			require.Equal(t, "priority", *result.ServiceTier)
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, "high", *result.ReasoningEffort)
		})
	}
}

func runOpenAIPartialUsageForward(t *testing.T, passthrough bool, upstreamBody string) (*OpenAIForwardResult, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"req_partial_usage"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
	account := &Account{
		ID:          9901,
		Name:        "partial-usage",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-partial-usage",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{
			"use_responses_api":  true,
			"openai_passthrough": passthrough,
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	body := []byte(`{"model":"gpt-5","stream":true,"input":"hello"}`)
	return svc.Forward(context.Background(), c, account, body)
}

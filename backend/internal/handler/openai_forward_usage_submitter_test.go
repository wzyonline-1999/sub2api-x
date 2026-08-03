package handler

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForwardUsageSubmitterIgnoresMissingResult(t *testing.T) {
	var calls atomic.Int32
	submitter := newOpenAIForwardUsageSubmitter(nil, func(*service.OpenAIForwardResult) {
		calls.Add(1)
	})

	submitter.Submit()
	require.Zero(t, calls.Load())
}

func TestOpenAIForwardUsageSubmitterSubmitsResultExactlyOnce(t *testing.T) {
	result := &service.OpenAIForwardResult{RequestID: "partial-usage"}
	var calls atomic.Int32
	var got atomic.Pointer[service.OpenAIForwardResult]
	submitter := newOpenAIForwardUsageSubmitter(result, func(value *service.OpenAIForwardResult) {
		got.Store(value)
		calls.Add(1)
	})

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			submitter.Submit()
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, calls.Load())
	require.Same(t, result, got.Load())
}

func TestOpenAIForwardUsageAttemptOutcome(t *testing.T) {
	forwardErr := errors.New("stream interrupted")
	tests := []struct {
		name              string
		result            *service.OpenAIForwardResult
		forwardErr        error
		commonCompletion  bool
		wantBeforeError   int32
		wantTotal         int32
		wantRecordable    bool
		wantCyberFallback bool
	}{
		{
			name:             "success records in common completion path",
			result:           &service.OpenAIForwardResult{RequestID: "success"},
			commonCompletion: true,
			wantTotal:        1,
			wantRecordable:   true,
		},
		{
			name:            "partial result records before error handling",
			result:          &service.OpenAIForwardResult{RequestID: "partial", Usage: service.OpenAIUsage{InputTokens: 7}},
			forwardErr:      forwardErr,
			wantBeforeError: 1,
			wantTotal:       1,
			wantRecordable:  true,
		},
		{
			name:             "partial image records before error handling",
			result:           &service.OpenAIForwardResult{RequestID: "partial-image", ImageCount: 1},
			forwardErr:       forwardErr,
			commonCompletion: true,
			wantBeforeError:  1,
			wantTotal:        1,
			wantRecordable:   true,
		},
		{
			name:              "empty partial result uses cyber fallback",
			result:            &service.OpenAIForwardResult{RequestID: "empty-partial"},
			forwardErr:        forwardErr,
			wantCyberFallback: true,
		},
		{
			name:              "error without result uses cyber fallback",
			forwardErr:        forwardErr,
			wantCyberFallback: true,
		},
		{
			name:              "client disconnect without observed usage is not charged",
			result:            &service.OpenAIForwardResult{RequestID: "client-gone", ClientDisconnect: true},
			forwardErr:        forwardErr,
			wantCyberFallback: true,
		},
		{
			name: "empty successful result records nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			submitter := newOpenAIForwardUsageSubmitter(tt.result, func(*service.OpenAIForwardResult) {
				calls.Add(1)
			})

			submitter.SubmitBeforeErrorHandling(tt.forwardErr)
			require.Equal(t, tt.wantBeforeError, calls.Load())
			if tt.commonCompletion {
				submitter.Submit()
			}

			require.Equal(t, tt.wantTotal, calls.Load())
			require.Equal(t, tt.wantRecordable, openAIForwardUsageIsRecordable(tt.forwardErr, tt.result))
			require.Equal(t, tt.wantCyberFallback, openAIForwardNeedsCyberUsage(tt.forwardErr, tt.result))
		})
	}
}

func TestOpenAIWSTurnPartialUsageDisposition(t *testing.T) {
	turnErr := errors.New("http bridge stream interrupted")
	partial := &service.OpenAIForwardResult{
		RequestID: "ws-partial-token",
		Usage:     service.OpenAIUsage{InputTokens: 9, OutputTokens: 2},
	}
	require.True(t, openAIForwardUsageIsRecordable(turnErr, partial))
	require.False(t, openAIForwardNeedsCyberUsage(turnErr, partial))
	require.False(t, openAIWSTurnSucceededForScheduling(turnErr, partial), "errored turn must never be reported as a scheduling success")
	require.True(t, openAIWSTurnShouldReportScheduling(turnErr, partial))
	require.False(t, openAIWSTurnShouldReportScheduling(&service.UpstreamFailoverError{StatusCode: 503}, partial), "outer failover handler owns the scheduling failure report")

	clientGoneWithoutUsage := &service.OpenAIForwardResult{
		RequestID:        "ws-client-gone",
		ClientDisconnect: true,
	}
	require.False(t, openAIForwardUsageIsRecordable(turnErr, clientGoneWithoutUsage))
	require.True(t, openAIForwardNeedsCyberUsage(turnErr, clientGoneWithoutUsage))
	require.False(t, openAIWSTurnSucceededForScheduling(turnErr, clientGoneWithoutUsage))
	require.False(t, openAIWSTurnShouldReportScheduling(turnErr, clientGoneWithoutUsage), "client disconnect must not penalize the selected account")
	require.False(t, shouldReportOpenAIWSProxyAccountFailureForAttempt(turnErr, true, false), "outer proxy error handling must also preserve client-disconnect neutrality")
	require.False(t, shouldReportOpenAIWSProxyAccountFailureForAttempt(turnErr, false, true), "outer proxy error handling must not duplicate an AfterTurn failure report")
	require.True(t, shouldReportOpenAIWSProxyAccountFailureForAttempt(turnErr, false, false), "outer proxy error handling owns unreported failures")
}

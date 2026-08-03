package handler

import (
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// openAIForwardUsageSubmitter owns the usage submission for one forwarding
// attempt. Forwarders may return both a result and an error, and some error
// paths later fall through to the common completion path (notably partial
// image results), so the attempt-level guard must make repeated Submit calls
// harmless.
type openAIForwardUsageSubmitter struct {
	result *service.OpenAIForwardResult
	submit func(*service.OpenAIForwardResult)
	once   sync.Once
}

func newOpenAIForwardUsageSubmitter(result *service.OpenAIForwardResult, submit func(*service.OpenAIForwardResult)) *openAIForwardUsageSubmitter {
	return &openAIForwardUsageSubmitter{result: result, submit: submit}
}

func (s *openAIForwardUsageSubmitter) Submit() {
	if s == nil || s.result == nil || s.submit == nil {
		return
	}
	s.once.Do(func() {
		s.submit(s.result)
	})
}

func (s *openAIForwardUsageSubmitter) SubmitBeforeErrorHandling(forwardErr error) {
	if forwardErr != nil && openAIForwardUsageIsRecordable(forwardErr, s.result) {
		s.Submit()
	}
}

// openAIForwardUsageIsRecordable reports whether this forwarding attempt owns
// a normal RecordUsage row. Successful results retain the existing behavior,
// while errored attempts require upstream-observed work so empty failover and
// client-close results are not charged.
func openAIForwardUsageIsRecordable(forwardErr error, result *service.OpenAIForwardResult) bool {
	return result != nil && (forwardErr == nil || service.HasObservedOpenAIUsageResult(result))
}

// openAIForwardNeedsCyberUsage reports whether the cyber fallback must create
// the usage row itself. Billable result+error partial responses are owned by
// the normal attempt submitter; empty error results remain cyber-owned.
func openAIForwardNeedsCyberUsage(forwardErr error, result *service.OpenAIForwardResult) bool {
	return forwardErr != nil && !openAIForwardUsageIsRecordable(forwardErr, result)
}

package service

// HasObservedOpenAIUsageResult reports whether a forwarding result contains
// upstream-metered work that must survive an accompanying forwarding error.
// A successful zero-usage response is still recorded by callers; this
// predicate is only for deciding ownership of result+error partial attempts.
func HasObservedOpenAIUsageResult(result *OpenAIForwardResult) bool {
	return result != nil && (result.Usage.HasObservedTokens() || result.ImageCount > 0 ||
		result.VideoCount > 0 || result.WebSearchCalls > 0 || result.SearchCount > 0 ||
		result.AudioUsage != nil)
}

package handler

import (
	"net/http"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// CountTokens returns a local Anthropic-compatible token estimate for OpenAI-backed groups.
func (h *OpenAIGatewayHandler) CountTokens(c *gin.Context) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.anthropicErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", buildRequestBodyReadErrorMessage(err))
		return
	}
	if len(body) == 0 {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if !gjson.ValidBytes(body) {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	setOpsRequestContext(c, "", false)
	c.JSON(http.StatusOK, gin.H{
		"input_tokens": estimateAnthropicInputTokens(body),
	})
}

func estimateAnthropicInputTokens(body []byte) int {
	total := 0

	system := gjson.GetBytes(body, "system")
	switch {
	case system.Type == gjson.String:
		total += estimateTextTokens(system.String())
	case system.IsArray():
		system.ForEach(func(_, block gjson.Result) bool {
			total += estimateTextTokens(block.Get("text").String())
			return true
		})
	}

	gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
		content := msg.Get("content")
		switch {
		case content.Type == gjson.String:
			total += estimateTextTokens(content.String())
		case content.IsArray():
			content.ForEach(func(_, block gjson.Result) bool {
				if text := block.Get("text").String(); text != "" {
					total += estimateTextTokens(text)
				} else if json := strings.TrimSpace(block.Raw); json != "" {
					total += estimateTextTokens(json)
				}
				return true
			})
		}
		return true
	})

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() {
		total += estimateTextTokens(tools.Raw)
	}

	if total <= 0 {
		total = estimateTextTokens(string(body))
	}
	if total <= 0 {
		return 1
	}
	return total
}

func estimateTextTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}
	if float64(ascii)/float64(len(runes)) >= 0.8 {
		return (len(runes) + 3) / 4
	}
	return len(runes)
}

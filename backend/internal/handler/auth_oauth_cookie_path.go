package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const oauthAPIRootPath = "/api/v1"

func oauthCookiePath(c *gin.Context, canonicalPath string) string {
	canonicalPath = normalizeOAuthCookiePath(canonicalPath)
	if canonicalPath == "/" {
		return canonicalPath
	}

	requestPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		requestPath = strings.TrimSpace(c.Request.URL.Path)
	}
	if requestPath == "" {
		return canonicalPath
	}

	if prefix, ok := requestPathPrefixBefore(requestPath, canonicalPath); ok {
		return prefix + canonicalPath
	}
	if strings.HasPrefix(canonicalPath, oauthAPIRootPath+"/") {
		if prefix, ok := requestPathPrefixBefore(requestPath, oauthAPIRootPath); ok {
			return prefix + canonicalPath
		}
	}
	return canonicalPath
}

func normalizeOAuthCookiePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	return path
}

func requestPathPrefixBefore(requestPath, marker string) (string, bool) {
	if marker == "" || marker == "/" {
		return "", false
	}
	searchFrom := 0
	for searchFrom < len(requestPath) {
		idx := strings.Index(requestPath[searchFrom:], marker)
		if idx < 0 {
			return "", false
		}
		idx += searchFrom
		after := idx + len(marker)
		if after < len(requestPath) && requestPath[after] != '/' {
			searchFrom = idx + 1
			continue
		}
		if idx == 0 {
			return "", true
		}
		prefix := strings.TrimRight(requestPath[:idx], "/")
		if prefix == "" {
			return "", true
		}
		return prefix, true
	}
	return "", false
}

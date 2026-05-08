package handler

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func (h *AuthHandler) frontendCallbackWithServerBasePath(callback string) string {
	basePath := ""
	if h != nil && h.cfg != nil {
		basePath = h.cfg.Server.BasePath
	}
	return frontendCallbackWithBasePath(callback, basePath)
}

func frontendCallbackWithBasePath(callback, basePath string) string {
	callback = strings.TrimSpace(callback)
	basePath = config.NormalizeBasePath(basePath)
	if callback == "" || basePath == "" {
		return callback
	}

	u, err := url.Parse(callback)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return callback
	}
	if u.Path == basePath || strings.HasPrefix(u.Path, basePath+"/") {
		return callback
	}

	u.Path = basePath + u.Path
	u.RawPath = ""
	return u.String()
}

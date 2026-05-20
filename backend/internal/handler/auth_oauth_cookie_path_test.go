package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuthCookiePathPreservesServerBasePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/sub2api/api/v1/auth/oauth/linuxdo/start", nil)

	require.Equal(t, "/sub2api/api/v1/auth/oauth", oauthCookiePath(c, "/api/v1/auth/oauth"))
	require.Equal(t, "/sub2api/api/v1/auth/oauth/linuxdo", oauthCookiePath(c, "/api/v1/auth/oauth/linuxdo"))
}

func TestOAuthCookiePathFallsBackToCanonicalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/start", nil)

	require.Equal(t, "/api/v1/auth/oauth", oauthCookiePath(c, "/api/v1/auth/oauth"))
	require.Equal(t, "/api/v1/auth/oauth/linuxdo", oauthCookiePath(c, "/api/v1/auth/oauth/linuxdo"))
}

func TestOAuthCookiePathUsesRequestAPIPrefixForLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/sub2api/api/v1/auth/logout", nil)

	require.Equal(t, "/sub2api/api/v1/auth/oauth", oauthCookiePath(c, "/api/v1/auth/oauth"))
	require.Equal(t, "/sub2api/api/v1/auth/oauth/wechat/payment", oauthCookiePath(c, "/api/v1/auth/oauth/wechat/payment"))
}

func TestOAuthCookiePathIgnoresPartialAPIPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/sub2api/api/v10/auth/logout", nil)

	require.Equal(t, "/api/v1/auth/oauth", oauthCookiePath(c, "/api/v1/auth/oauth"))
}

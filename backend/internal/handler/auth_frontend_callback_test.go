package handler

import "testing"

func TestFrontendCallbackWithBasePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		callback string
		basePath string
		want     string
	}{
		{
			name:     "relative absolute path gets prefixed",
			callback: "/auth/oauth/callback",
			basePath: "/sub2api",
			want:     "/sub2api/auth/oauth/callback",
		},
		{
			name:     "query and fragment are preserved",
			callback: "/auth/oauth/callback?provider=google#token=abc",
			basePath: "/sub2api/",
			want:     "/sub2api/auth/oauth/callback?provider=google#token=abc",
		},
		{
			name:     "already prefixed path is unchanged",
			callback: "/sub2api/auth/oauth/callback",
			basePath: "/sub2api",
			want:     "/sub2api/auth/oauth/callback",
		},
		{
			name:     "absolute URL is unchanged",
			callback: "https://app.example.com/auth/oauth/callback",
			basePath: "/sub2api",
			want:     "https://app.example.com/auth/oauth/callback",
		},
		{
			name:     "network path is unchanged",
			callback: "//app.example.com/auth/oauth/callback",
			basePath: "/sub2api",
			want:     "//app.example.com/auth/oauth/callback",
		},
		{
			name:     "no base path leaves callback unchanged",
			callback: "/auth/oauth/callback",
			basePath: "",
			want:     "/auth/oauth/callback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := frontendCallbackWithBasePath(tt.callback, tt.basePath); got != tt.want {
				t.Fatalf("frontendCallbackWithBasePath(%q, %q) = %q, want %q", tt.callback, tt.basePath, got, tt.want)
			}
		})
	}
}

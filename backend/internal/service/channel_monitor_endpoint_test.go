//go:build unit

package service

import "testing"

func TestChannelMonitorJoinURLSupportsBasePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "origin only",
			base: "https://api.example.com",
			path: "/v1/chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "reverse proxy prefix",
			base: "https://api.example.com/sub2api",
			path: "/v1/chat/completions",
			want: "https://api.example.com/sub2api/v1/chat/completions",
		},
		{
			name: "prefix already includes v1",
			base: "https://api.example.com/sub2api/v1",
			path: "/v1/chat/completions",
			want: "https://api.example.com/sub2api/v1/chat/completions",
		},
		{
			name: "gemini prefix already includes v1beta",
			base: "https://api.example.com/google/v1beta",
			path: "/v1beta/models/gemini-2.5-pro:generateContent",
			want: "https://api.example.com/google/v1beta/models/gemini-2.5-pro:generateContent",
		},
		{
			name: "already full path",
			base: "https://api.example.com/v1/chat/completions",
			path: "/v1/chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := joinURL(tt.base, tt.path); got != tt.want {
				t.Fatalf("joinURL(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeEndpointKeepsBasePath(t *testing.T) {
	t.Parallel()

	got := normalizeEndpoint(" https://api.example.com/sub2api/v1/ ")
	want := "https://api.example.com/sub2api/v1"
	if got != want {
		t.Fatalf("normalizeEndpoint() = %q, want %q", got, want)
	}
}

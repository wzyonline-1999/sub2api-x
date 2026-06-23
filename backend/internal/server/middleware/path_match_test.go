package middleware

import "testing"

func TestRequestPathHasMountedPrefixUsesSegmentBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		want   bool
	}{
		{
			name:   "root mounted v1beta",
			path:   "/v1beta/models",
			prefix: "/v1beta",
			want:   true,
		},
		{
			name:   "sub path mounted v1beta",
			path:   "/sub2api/v1beta/models",
			prefix: "/v1beta",
			want:   true,
		},
		{
			name:   "multi segment mounted antigravity",
			path:   "/sub2api/admin/antigravity/v1beta/models",
			prefix: "/antigravity/v1beta",
			want:   true,
		},
		{
			name:   "partial v1beta segment does not match",
			path:   "/sub2api/v1betamax/models",
			prefix: "/v1beta",
			want:   false,
		},
		{
			name:   "partial antigravity segment does not match",
			path:   "/sub2api/antigravity-v1beta/models",
			prefix: "/antigravity/v1beta",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestPathHasMountedPrefix(tt.path, tt.prefix); got != tt.want {
				t.Fatalf("requestPathHasMountedPrefix(%q, %q) = %v, want %v", tt.path, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestRequestPathEqualsMountedUsesEndpointSuffix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		endpoint string
		want     bool
	}{
		{
			name:     "root mounted usage",
			path:     "/v1/usage",
			endpoint: "/v1/usage",
			want:     true,
		},
		{
			name:     "sub path mounted usage",
			path:     "/sub2api/v1/usage",
			endpoint: "/v1/usage",
			want:     true,
		},
		{
			name:     "multi segment mounted usage",
			path:     "/sub2api/admin/v1/usage",
			endpoint: "/v1/usage",
			want:     true,
		},
		{
			name:     "extra segment after endpoint does not match",
			path:     "/sub2api/v1/usage/extra",
			endpoint: "/v1/usage",
			want:     false,
		},
		{
			name:     "partial endpoint segment does not match",
			path:     "/sub2api/v1/usage-extra",
			endpoint: "/v1/usage",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestPathEqualsMounted(tt.path, tt.endpoint); got != tt.want {
				t.Fatalf("requestPathEqualsMounted(%q, %q) = %v, want %v", tt.path, tt.endpoint, got, tt.want)
			}
		})
	}
}

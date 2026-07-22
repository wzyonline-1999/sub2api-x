package googleapi

import (
	"net/http"
	"testing"
)

func TestHTTPStatusToGoogleStatusServiceUnavailable(t *testing.T) {
	if got := HTTPStatusToGoogleStatus(http.StatusServiceUnavailable); got != "UNAVAILABLE" {
		t.Fatalf("status = %q, want UNAVAILABLE", got)
	}
}

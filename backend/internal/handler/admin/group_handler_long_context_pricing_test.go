package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateGroupRequestLongContextPricingDefault(t *testing.T) {
	t.Run("omitted defaults to enabled", func(t *testing.T) {
		var req CreateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
		require.Nil(t, req.LongContextPricingEnabled)
		require.True(t, req.longContextPricingEnabled())
	})

	t.Run("explicit false is preserved", func(t *testing.T) {
		var req CreateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"long_context_pricing_enabled":false}`), &req))
		require.NotNil(t, req.LongContextPricingEnabled)
		require.False(t, req.longContextPricingEnabled())
	})
}

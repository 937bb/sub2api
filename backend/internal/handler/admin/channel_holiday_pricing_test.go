//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelHolidayPricingAPIRoundTrip(t *testing.T) {
	var req channelTimePricingRequest
	require.NoError(t, json.Unmarshal([]byte(`{"timezone":"Asia/Shanghai","weekdays_only":true,"excluded_dates":["2026-10-01"],"periods":[{"start_time":"09:00:00","end_time":"12:00:00","multiplier":2}]}`), &req))
	value := timePricingRequestToService(&req)
	require.Equal(t, []string{"2026-10-01"}, value.ExcludedDates)
	response := timePricingToResponse(value)
	require.Equal(t, value.ExcludedDates, response.ExcludedDates)
	value.ExcludedDates[0] = "2026-10-02"
	require.Equal(t, "2026-10-01", req.ExcludedDates[0])
	require.Equal(t, "2026-10-01", response.ExcludedDates[0])
}

//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelHolidayPricingBoundaries(t *testing.T) {
	config := &ChannelTimePricing{Timezone: "Asia/Shanghai", WeekdaysOnly: true,
		ExcludedDates: []string{"2026-10-01", "2026-10-02", "2026-10-07"},
		Periods: []ChannelTimePricingPeriod{
			{StartTime: "09:00:00", EndTime: "12:00:00", Multiplier: 2},
			{StartTime: "14:00:00", EndTime: "18:00:00", Multiplier: 2},
		}}
	for _, tt := range []struct {
		at   string
		want float64
	}{
		{"2026-09-30T16:00:00Z", 1}, // Local holiday midnight.
		{"2026-10-01T01:00:00Z", 1},
		{"2026-10-02T08:00:00Z", 1},
		{"2026-10-07T01:00:00Z", 1},
		{"2026-10-08T00:59:59Z", 1},
		{"2026-10-08T01:00:00Z", 2},
		{"2026-10-08T04:00:00Z", 1},
		{"2026-10-08T06:00:00Z", 2},
		{"2026-10-08T10:00:00Z", 1},
		{"2026-10-10T01:00:00Z", 1}, // Saturday, even when a makeup workday.
		{"2027-10-01T01:00:00Z", 2}, // A date does not silently repeat next year.
	} {
		t.Run(tt.at, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tt.at)
			require.NoError(t, err)
			require.Equal(t, tt.want, config.MultiplierAt(at))
		})
	}
	for _, invalid := range []string{"10-01", "2026-2-01", "2026-02-29", "2026-13-01", "0000-01-01"} {
		config.ExcludedDates = []string{invalid}
		require.Error(t, validateChannelTimePricing(config), invalid)
	}
	config.ExcludedDates = []string{"2028-02-29"}
	require.NoError(t, validateChannelTimePricing(config))
}

func TestHolidayPricingCloneAndSchedule(t *testing.T) {
	card := ChannelModelPricing{TimePricing: &ChannelTimePricing{
		Timezone: "Asia/Shanghai", WeekdaysOnly: true,
		ExcludedDates: []string{"2026-01-05"},
		Periods:       []ChannelTimePricingPeriod{{StartTime: "09:00:00", EndTime: "09:00:01", Multiplier: 2}},
	}}
	cloned := card.Clone()
	cloned.TimePricing.ExcludedDates[0] = "2026-01-06"
	require.Equal(t, "2026-01-05", card.TimePricing.ExcludedDates[0])
	resolved := &ResolvedPricing{Source: PricingSourceChannel, channelPricing: &card}
	schedule := resolvedTimePricingSchedule(resolved)
	require.NotNil(t, schedule)
	require.Len(t, schedule.Periods, 1)
	require.Equal(t, 2.0, schedule.Periods[0].Multiplier)
	require.Equal(t, card.TimePricing.ExcludedDates, schedule.ExcludedDates)
	schedule.ExcludedDates[0] = "2026-01-07"
	require.Equal(t, "2026-01-05", card.TimePricing.ExcludedDates[0])
}

func TestDeepSeekHolidayPricingActualCharge(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, "deepseek"} {
		cards := []ChannelModelPricing{{Platform: platform, Models: []string{"deepseek-flash"}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(4e-6), CacheReadPrice: testPtrFloat64(.02e-6),
			TimePricing: &ChannelTimePricing{Timezone: "Asia/Shanghai", WeekdaysOnly: true, ExcludedDates: []string{"2026-10-01"},
				Periods: []ChannelTimePricingPeriod{{StartTime: "09:00:00", EndTime: "12:00:00", Multiplier: 2}}},
		}}
		bs, resolver := newTokenCostTestEnv(t, platform, cards, nil)
		group := enabledGroup(platform)
		for _, tc := range []struct {
			at         string
			multiplier float64
		}{{"2026-10-01T02:00:00Z", 1}, {"2026-10-08T02:00:00Z", 2}} {
			at, err := time.Parse(time.RFC3339, tc.at)
			require.NoError(t, err)
			cost, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
				Ctx: context.Background(), Model: "deepseek-flash", Group: group, Resolver: resolver,
				PricingAt: at, RateMultiplier: .3,
				Tokens: UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000},
			})
			require.NoError(t, err)
			require.InDelta(t, 5.02*tc.multiplier*.3, cost.ActualCost, 1e-9, platform+tc.at)
		}
	}
}

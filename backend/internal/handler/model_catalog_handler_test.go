//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildModelCatalogFiltersGroupsAndCalculatesEffectiveRate(t *testing.T) {
	baseInput := 0.000003
	baseOutput := 0.000012
	groups := []service.Group{
		{
			ID: 1, Name: "public", Platform: "openai", RateMultiplier: 1.2,
			SubscriptionType: service.SubscriptionTypeStandard,
		},
		{
			ID: 2, Name: "pro", Platform: "openai", RateMultiplier: 1.5,
			IsExclusive: true, SubscriptionType: service.SubscriptionTypeStandard,
			TimeBillingRules: []domain.TimeBillingRule{{
				ID: "night", Enabled: true, RepeatType: domain.TimeBillingRepeatDaily,
				Start: "00:00", End: "23:00", RateMultiplier: 2,
			}},
		},
	}
	rows := buildModelCatalog(
		[]service.AvailableChannel{{
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{{ID: 1, Platform: "openai"}, {ID: 2, Platform: "openai"}, {ID: 99, Platform: "openai"}},
			SupportedModels: []service.SupportedModel{{
				Name: "gpt-test", Platform: "openai",
				Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &baseInput, OutputPrice: &baseOutput},
			}},
		}},
		groups,
		map[int64]float64{2: 0.8},
		time.Date(2026, time.June, 29, 12, 0, 0, 0, time.UTC),
	)

	require.Len(t, rows, 1)
	require.Equal(t, "gpt-test", rows[0].Name)
	require.Len(t, rows[0].Groups, 2)

	var pro modelCatalogGroup
	for _, group := range rows[0].Groups {
		if group.ID == 2 {
			pro = group
		}
	}
	require.Equal(t, 0.8, pro.ResolvedRateMultiplier)
	require.Equal(t, 2.0, pro.TimeRateMultiplier)
	require.Equal(t, 1.6, pro.EffectiveRateMultiplier)
	require.NotNil(t, pro.Pricing)
	require.InDelta(t, baseInput*1.6, *pro.Pricing.InputPrice, 1e-12)
	require.InDelta(t, baseOutput*1.6, *pro.Pricing.OutputPrice, 1e-12)
}

func TestBuildModelCatalogDeduplicatesModelAcrossChannels(t *testing.T) {
	pricing := &service.ChannelModelPricing{BillingMode: service.BillingModePerRequest}
	channels := []service.AvailableChannel{
		{Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 1, Platform: "openai"}}, SupportedModels: []service.SupportedModel{{Name: "same", Platform: "openai", Pricing: pricing}}},
		{Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 1, Platform: "openai"}}, SupportedModels: []service.SupportedModel{{Name: "SAME", Platform: "openai", Pricing: pricing}}},
	}
	rows := buildModelCatalog(channels, []service.Group{{ID: 1, Name: "public", Platform: "openai", RateMultiplier: 1}}, nil, time.Now())
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].ChannelCount)
	require.Len(t, rows[0].Groups, 1)
}

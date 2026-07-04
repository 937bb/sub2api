package admin

import (
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestSanitizeAdminPaymentOrderForResponseDerivesCurrencyAndOmitsProviderSnapshot(t *testing.T) {
	order := &dbent.PaymentOrder{
		ID:               123,
		UserID:           456,
		Amount:           10,
		PayAmount:        12.34,
		FeeRate:          2.5,
		PaymentType:      "stripe",
		OutTradeNo:       "sub2_test_order",
		Status:           "COMPLETED",
		OrderType:        "balance",
		ProviderSnapshot: map[string]any{"currency": "hkd", "merchant_id": "secret_merchant"},
	}

	sanitized := sanitizeAdminPaymentOrderForResponse(order)
	require.NotNil(t, sanitized)
	require.Equal(t, "HKD", sanitized.Currency)

	raw, err := json.Marshal(sanitized)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"currency":"HKD"`)
	require.NotContains(t, string(raw), "provider_snapshot")
	require.NotContains(t, string(raw), "secret_merchant")
}

func TestSanitizeAdminPaymentOrdersForResponseDerivesListCurrencyAndOmitsProviderSnapshot(t *testing.T) {
	orders := []*dbent.PaymentOrder{
		{
			ID:               1,
			UserID:           10,
			Amount:           100,
			PayAmount:        100,
			PaymentType:      "airwallex",
			OutTradeNo:       "sub2_jpy_order",
			Status:           "COMPLETED",
			OrderType:        "subscription",
			ProviderSnapshot: map[string]any{"currency": "JPY", "merchant_id": "secret_airwallex"},
		},
		{
			ID:               2,
			UserID:           20,
			Amount:           20,
			PayAmount:        20.5,
			PaymentType:      "stripe",
			OutTradeNo:       "sub2_usd_order",
			Status:           "COMPLETED",
			OrderType:        "balance",
			ProviderSnapshot: map[string]any{"currency": "usd", "merchant_id": "secret_stripe"},
		},
	}

	sanitized := sanitizeAdminPaymentOrdersForResponse(orders)
	require.Len(t, sanitized, 2)
	require.Equal(t, "JPY", sanitized[0].Currency)
	require.Equal(t, "USD", sanitized[1].Currency)

	raw, err := json.Marshal(sanitized)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"currency":"JPY"`)
	require.Contains(t, string(raw), `"currency":"USD"`)
	require.NotContains(t, string(raw), "provider_snapshot")
	require.NotContains(t, string(raw), "secret_airwallex")
	require.NotContains(t, string(raw), "secret_stripe")
}

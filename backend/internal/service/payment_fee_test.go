//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestEffectiveRechargeFeeRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		paymentType string
		want        float64
	}{
		{name: "alipay keeps configured fee", paymentType: payment.TypeAlipay, want: 1.6},
		{name: "tron has no fee", paymentType: payment.TypeUSDTTron, want: 0},
		{name: "bep20 has no fee", paymentType: payment.TypeUSDTBEP20, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := EffectiveRechargeFeeRate(tt.paymentType, 1.6); got != tt.want {
				t.Fatalf("EffectiveRechargeFeeRate(%q, 1.6) = %v, want %v", tt.paymentType, got, tt.want)
			}
		})
	}
}

func TestUSDTRechargeUsesCNYFaceValueAndRetainsExplicitBonus(t *testing.T) {
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 0.14}
	for _, method := range []string{payment.TypeUSDTTron, payment.TypeUSDTBEP20} {
		require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 100}, quoteRechargeBonusForPaymentType(cfg, 100, "CNY", method))
	}
	cfg.RechargeBonusTiers = []RechargeBonusTier{{MinAmount: 100, BonusPercent: 10}}
	require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 110, Bonus: 10, Percent: 10}, quoteRechargeBonusForPaymentType(cfg, 100, "CNY", payment.TypeUSDTTron))
	require.Equal(t, 0.14, cfg.BalanceRechargeMultiplier)
	require.InDelta(t, 15.4, quoteRechargeBonusForPaymentType(cfg, 100, "CNY", payment.TypeAlipay).Credited, 0.000001)
}

func TestUSDTQuoteRejectsInvalidAmountBeforeAccessingProvider(t *testing.T) {
	service := &PaymentService{}
	for _, amount := range []float64{0, -1, 1.234, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := service.GetUSDTQuote(context.Background(), amount, payment.TypeUSDTTron)
		require.Error(t, err)
	}
}

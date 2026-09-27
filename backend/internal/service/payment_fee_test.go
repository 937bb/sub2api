//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
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

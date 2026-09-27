//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestApplyPaymentMethodFeeRates(t *testing.T) {
	t.Parallel()

	methods := map[string]service.MethodLimits{
		payment.TypeAlipay:    {},
		payment.TypeUSDTTron:  {},
		payment.TypeUSDTBEP20: {},
	}

	applyPaymentMethodFeeRates(methods, 1.6)

	if got := methods[payment.TypeAlipay].FeeRate; got != 1.6 {
		t.Fatalf("alipay fee rate = %v, want 1.6", got)
	}
	if got := methods[payment.TypeUSDTTron].FeeRate; got != 0 {
		t.Fatalf("TRON fee rate = %v, want 0", got)
	}
	if got := methods[payment.TypeUSDTBEP20].FeeRate; got != 0 {
		t.Fatalf("BEP20 fee rate = %v, want 0", got)
	}
}

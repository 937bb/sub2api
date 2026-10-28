package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// EffectiveRechargeFeeRate returns the fee charged by a checkout method.
// On-chain USDT payments are quoted directly and never inherit the fiat fee.
func EffectiveRechargeFeeRate(paymentType string, configuredRate float64) float64 {
	switch strings.TrimSpace(paymentType) {
	case payment.TypeUSDTTron, payment.TypeUSDTBEP20:
		return 0
	default:
		return configuredRate
	}
}

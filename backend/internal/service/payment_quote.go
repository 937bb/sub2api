package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// GetUSDTQuote returns a live CNY-to-USDT quote without creating an order.
func (s *PaymentService) GetUSDTQuote(ctx context.Context, amount float64, paymentType string) (*payment.USDTQuoteResponse, error) {
	paymentType = NormalizeVisibleMethod(paymentType)
	if paymentType != payment.TypeUSDTTron && paymentType != payment.TypeUSDTBEP20 {
		return nil, infraerrors.BadRequest("INVALID_USDT_PAYMENT_TYPE", "invalid USDT payment type")
	}
	decimalAmount := decimal.NewFromFloat(amount)
	if !decimalAmount.IsPositive() || !decimalAmount.Equal(decimalAmount.Round(2)) {
		return nil, infraerrors.BadRequest("INVALID_USDT_QUOTE_AMOUNT", "USDT quote amount must be positive with at most 2 decimal places")
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	sel, err := s.loadBalancer.SelectInstance(ctx, "", paymentType, payment.Strategy(cfg.LoadBalanceStrategy), amount)
	if err != nil || sel == nil {
		return nil, infraerrors.ServiceUnavailable("USDT_QUOTE_UNAVAILABLE", "live USDT quote is temporarily unavailable")
	}
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Error("[PaymentService] create USDT quote provider failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("USDT_QUOTE_UNAVAILABLE", "live USDT quote is temporarily unavailable")
	}
	quoter, ok := prov.(payment.USDTQuoteProvider)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("USDT_QUOTE_UNAVAILABLE", "selected payment provider does not support live USDT quotes")
	}
	quote, err := quoter.QuoteUSDT(ctx, payment.USDTQuoteRequest{
		Amount:      decimalAmount.StringFixed(2),
		PaymentType: paymentType,
	})
	if err != nil {
		slog.Warn("[PaymentService] live USDT quote failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "payment_type", paymentType, "error", err)
		return nil, infraerrors.ServiceUnavailable("USDT_QUOTE_UNAVAILABLE", "live USDT quote is temporarily unavailable")
	}
	if quote == nil || quote.Amount <= 0 || quote.Rate <= 0 || quote.QuotedAmount <= 0 ||
		!strings.EqualFold(quote.Currency, "CNY") || !strings.EqualFold(quote.Token, "USDT") {
		return nil, infraerrors.ServiceUnavailable("USDT_QUOTE_UNAVAILABLE", fmt.Sprintf("invalid live USDT quote for %s", paymentType))
	}
	return quote, nil
}

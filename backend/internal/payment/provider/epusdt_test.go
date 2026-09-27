package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestEpusdtSignatureMatchesGMPayV2Vector(t *testing.T) {
	signature := epusdtSignature(map[string]any{
		"signature": "ignored",
		"pid":       "1000",
		"empty":     "",
		"nil":       nil,
		"name":      "VIP",
		"amount":    json.Number("100.0"),
	}, "test-secret")
	require.Equal(t, "ced9141fab53a83d1178f903e7c22d8a2a7033520f31e319934e92455a008b6c", signature)
}

func TestEpusdtCreatePaymentUsesRequestedNetworkAndLocksUSDT(t *testing.T) {
	for _, testCase := range []struct {
		paymentType string
		network     string
		address     string
	}{
		{payment.TypeUSDTTron, "tron", "TAdBr8W7soSSLnBSJSB8dbg1nUyTWsB4dF"},
		{payment.TypeUSDTBEP20, "binance", "0x1dafac91abe2ee53b2d2023b8225b4c293a4f884"},
	} {
		t.Run(testCase.network, func(t *testing.T) {
			secret := "merchant-secret"
			quoteAt := time.Now().Unix()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, gmPayCreatePath, r.URL.Path)
				var body map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&body))
				require.Equal(t, testCase.network, body["network"])
				require.Equal(t, "cny", body["currency"])
				require.Equal(t, "usdt", body["token"])
				require.Equal(t, epusdtRatePolicyLive, body["rate_policy"])
				signature := body["signature"]
				delete(body, "signature")
				require.Equal(t, epusdtSignature(body, secret), signature)
				_, _ = fmt.Fprintf(w, `{"status_code":200,"message":"success","data":{"trade_id":"trade-1","order_id":"sub2_order","amount":100.00,"actual_amount":13.827451,"receive_address":%q,"token":"usdt","network":%q,"status":1,"payment_url":"https://pay.example.test/cashier/trade-1","rate_policy":"live","locked_rate":0.13827451,"quoted_amount":13.827451,"amount_precision":6,"rate_source":"https://api.coingecko.com/api/v3/simple/price?ids=tether","rate_quote_at":%d}}`, testCase.address, testCase.network, quoteAt)
			}))
			defer server.Close()

			provider := newTestEpusdt(t, server.URL, secret)
			result, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID:     "sub2_order",
				Amount:      "100.00",
				PaymentType: testCase.paymentType,
				Subject:     "Sub2API 100.00 CNY",
				ReturnURL:   "https://merchant.example.test/payment/result",
			})
			require.NoError(t, err)
			require.Equal(t, "trade-1", result.TradeNo)
			require.Equal(t, "https://pay.example.test/cashier/trade-1", result.PayURL)
			require.Equal(t, testCase.address, result.QRCode)
			require.Equal(t, testCase.address, result.PaymentAddress)
			require.Equal(t, "13.827451", result.PaymentTokenAmount)
			require.Equal(t, "USDT", result.PaymentToken)
			require.Equal(t, testCase.network, result.PaymentNetwork)
		})
	}
}

func TestEpusdtQuoteUSDTUsesSignedLiveQuoteEndpoint(t *testing.T) {
	secret := "merchant-secret"
	quoteAt := time.Now().Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, gmPayQuotePath, r.URL.Path)
		var body map[string]any
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&body))
		require.Equal(t, "binance", body["network"])
		require.Equal(t, json.Number("500.00"), body["amount"])
		signature := body["signature"]
		delete(body, "signature")
		require.Equal(t, epusdtSignature(body, secret), signature)
		_, _ = fmt.Fprintf(w, `{"status_code":200,"message":"success","data":{"currency":"CNY","token":"USDT","network":"binance","amount":500.00,"rate_policy":"live","rate":0.14898586,"quoted_amount":74.492930,"amount_precision":6,"rate_source":"https://api.coingecko.com/api/v3/simple/price?ids=tether","rate_quote_at":%d}}`, quoteAt)
	}))
	defer server.Close()

	provider := newTestEpusdt(t, server.URL, secret)
	result, err := provider.QuoteUSDT(context.Background(), payment.USDTQuoteRequest{
		Amount: "500.00", PaymentType: payment.TypeUSDTBEP20,
	})
	require.NoError(t, err)
	require.Equal(t, "binance", result.Network)
	require.Equal(t, 500.0, result.Amount)
	require.InDelta(t, 0.14898586, result.Rate, 1e-12)
	require.Equal(t, 74.49293, result.QuotedAmount)
	require.Equal(t, 6, result.AmountPrecision)
}

func TestEpusdtCreatePaymentRejectsMismatchedCNYAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"trade-1","order_id":"sub2_order","amount":99.99,"actual_amount":13.8,"receive_address":"TAdBr8W7soSSLnBSJSB8dbg1nUyTWsB4dF","token":"usdt","network":"tron","payment_url":"https://pay.example.test/cashier/trade-1"}}`))
	}))
	defer server.Close()

	provider := newTestEpusdt(t, server.URL, "merchant-secret")
	_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order", Amount: "100.00", PaymentType: payment.TypeUSDTTron,
	})
	require.ErrorContains(t, err, "CNY amount mismatch")
}

func TestEpusdtCreatePaymentRejectsGatewayWithoutLiveRateProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"trade-1","order_id":"sub2_order","amount":100.00,"actual_amount":13.8,"receive_address":"TAdBr8W7soSSLnBSJSB8dbg1nUyTWsB4dF","token":"usdt","network":"tron","payment_url":"https://pay.example.test/cashier/trade-1"}}`))
	}))
	defer server.Close()

	provider := newTestEpusdt(t, server.URL, "merchant-secret")
	_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order", Amount: "100.00", PaymentType: payment.TypeUSDTTron,
	})
	require.ErrorContains(t, err, "live rate policy was not honored")
}

func TestEpusdtQueryOrderReturnsOriginalCNYAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, gmPayQueryPath+"trade-1", r.URL.Path)
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"trade-1","amount":100.00,"actual_amount":13.827451,"token":"USDT","network":"tron","status":2,"payment_url":"https://pay.example.test/cashier/trade-1"}}`))
	}))
	defer server.Close()

	provider := newTestEpusdt(t, server.URL, "merchant-secret")
	result, err := provider.QueryOrder(context.Background(), "trade-1")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, result.Status)
	require.Equal(t, 100.0, result.Amount)
	require.Equal(t, "1000", result.Metadata["pid"])
	require.Equal(t, "USDT", result.Metadata["token"])
	require.Equal(t, "2", result.Metadata["status"])
}

func TestEpusdtVerifyNotification(t *testing.T) {
	provider := newTestEpusdt(t, "https://gateway.example.test", "merchant-secret")
	payload := map[string]any{
		"pid":                  "1000",
		"trade_id":             "trade-1",
		"order_id":             "sub2_order",
		"amount":               json.Number("100.0"),
		"actual_amount":        json.Number("13.827451"),
		"receive_address":      "TAddress",
		"token":                "USDT",
		"network":              "tron",
		"block_transaction_id": "tx-1",
		"status":               json.Number("2"),
	}
	payload["signature"] = epusdtSignature(payload, "merchant-secret")
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	notification, err := provider.VerifyNotification(context.Background(), string(raw), nil)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
	require.Equal(t, "sub2_order", notification.OrderID)
	require.Equal(t, "trade-1", notification.TradeNo)
	require.Equal(t, 100.0, notification.Amount)
	require.Equal(t, "CNY", notification.Metadata["currency"])

	payload["amount"] = json.Number("99")
	tampered, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = provider.VerifyNotification(context.Background(), string(tampered), nil)
	require.ErrorContains(t, err, "signature")
}

func TestEpusdtVerifyNotificationRejectsWrongPID(t *testing.T) {
	provider := newTestEpusdt(t, "https://gateway.example.test", "merchant-secret")
	payload := map[string]any{
		"pid":           "2000",
		"trade_id":      "trade-1",
		"order_id":      "sub2_order",
		"amount":        json.Number("100"),
		"actual_amount": json.Number("13.8"),
		"token":         "USDT",
		"network":       "tron",
		"status":        json.Number("2"),
	}
	payload["signature"] = epusdtSignature(payload, "merchant-secret")
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = provider.VerifyNotification(context.Background(), string(raw), nil)
	require.ErrorContains(t, err, "pid mismatch")
}

func TestEpusdtRefundIsDisabled(t *testing.T) {
	provider := newTestEpusdt(t, "https://gateway.example.test", "merchant-secret")
	_, err := provider.Refund(context.Background(), payment.RefundRequest{})
	require.ErrorContains(t, err, "disabled")
}

func TestCreateProviderBuildsEpusdt(t *testing.T) {
	created, err := CreateProvider(payment.TypeEpusdt, "1", map[string]string{
		"pid":       "1000",
		"secret":    "merchant-secret",
		"apiBase":   "http://127.0.0.1:8000",
		"notifyUrl": "https://merchant.example.test/api/v1/payment/webhook/epusdt",
	})
	require.NoError(t, err)
	require.IsType(t, &Epusdt{}, created)
	require.Equal(t, []payment.PaymentType{payment.TypeUSDTTron, payment.TypeUSDTBEP20}, created.SupportedTypes())
}

func newTestEpusdt(t *testing.T, apiBase, secret string) *Epusdt {
	t.Helper()
	provider, err := NewEpusdt("1", map[string]string{
		"pid":       "1000",
		"secret":    secret,
		"apiBase":   apiBase,
		"notifyUrl": "https://merchant.example.test/api/v1/payment/webhook/epusdt",
	})
	require.NoError(t, err)
	return provider
}

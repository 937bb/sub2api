package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	epusdtHTTPTimeout     = 15 * time.Second
	epusdtMaxResponseSize = 1 << 20
	epusdtMaxLiveQuoteAge = 3 * time.Minute
	epusdtMaxFutureSkew   = 30 * time.Second
	epusdtRatePolicyLive  = "live"
	epusdtStatusPaid      = 2
	epusdtStatusExpired   = 3
	gmPayQuotePath        = "/payments/gmpay/v1/order/quote"
	gmPayCreatePath       = "/payments/gmpay/v1/order/create-transaction"
	gmPayQueryPath        = "/pay/checkout-counter-resp/"
)

type Epusdt struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type epusdtAPIResponse struct {
	StatusCode json.Number     `json:"status_code"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
}

type epusdtOrder struct {
	PID                string      `json:"pid"`
	TradeID            string      `json:"trade_id"`
	OrderID            string      `json:"order_id"`
	Amount             json.Number `json:"amount"`
	ActualAmount       json.Number `json:"actual_amount"`
	ReceiveAddress     string      `json:"receive_address"`
	Currency           string      `json:"currency"`
	Token              string      `json:"token"`
	Network            string      `json:"network"`
	BlockTransactionID string      `json:"block_transaction_id"`
	Status             json.Number `json:"status"`
	PaymentURL         string      `json:"payment_url"`
	ExpirationTime     int64       `json:"expiration_time"`
	Signature          string      `json:"signature"`
	RatePolicy         string      `json:"rate_policy"`
	LockedRate         json.Number `json:"locked_rate"`
	QuotedAmount       json.Number `json:"quoted_amount"`
	AmountPrecision    int         `json:"amount_precision"`
	RateSource         string      `json:"rate_source"`
	RateQuoteAt        int64       `json:"rate_quote_at"`
}

type epusdtQuote struct {
	Currency        string      `json:"currency"`
	Token           string      `json:"token"`
	Network         string      `json:"network"`
	Amount          json.Number `json:"amount"`
	RatePolicy      string      `json:"rate_policy"`
	Rate            json.Number `json:"rate"`
	QuotedAmount    json.Number `json:"quoted_amount"`
	AmountPrecision int         `json:"amount_precision"`
	RateSource      string      `json:"rate_source"`
	RateQuoteAt     int64       `json:"rate_quote_at"`
}

func NewEpusdt(instanceID string, config map[string]string) (*Epusdt, error) {
	for _, key := range []string{"pid", "secret", "apiBase", "notifyUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("epusdt config missing required key: %s", key)
		}
	}
	apiBase, err := normalizeEpusdtAPIBase(config["apiBase"])
	if err != nil {
		return nil, err
	}
	notifyURL, err := url.Parse(strings.TrimSpace(config["notifyUrl"]))
	if err != nil || notifyURL.Scheme == "" || notifyURL.Host == "" {
		return nil, fmt.Errorf("epusdt notifyUrl must be an absolute HTTP(S) URL")
	}
	if notifyURL.Scheme != "http" && notifyURL.Scheme != "https" {
		return nil, fmt.Errorf("epusdt notifyUrl must be an absolute HTTP(S) URL")
	}
	cfg := make(map[string]string, len(config))
	for key, value := range config {
		cfg[key] = value
	}
	cfg["apiBase"] = apiBase
	return &Epusdt{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: epusdtHTTPTimeout},
	}, nil
}

func normalizeEpusdtAPIBase(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("epusdt apiBase must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("epusdt apiBase must be an absolute HTTP(S) URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (e *Epusdt) Name() string        { return "Epusdt" }
func (e *Epusdt) ProviderKey() string { return payment.TypeEpusdt }
func (e *Epusdt) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeUSDTTron, payment.TypeUSDTBEP20}
}

func (e *Epusdt) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"pid": strings.TrimSpace(e.config["pid"])}
}

func (e *Epusdt) QuoteUSDT(ctx context.Context, req payment.USDTQuoteRequest) (*payment.USDTQuoteResponse, error) {
	network, err := epusdtNetworkForPaymentType(req.PaymentType)
	if err != nil {
		return nil, err
	}
	amount, err := epusdtAmount(req.Amount)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"pid":         strings.TrimSpace(e.config["pid"]),
		"currency":    "cny",
		"token":       "usdt",
		"network":     network,
		"amount":      json.Number(amount.StringFixed(2)),
		"rate_policy": epusdtRatePolicyLive,
	}
	params["signature"] = epusdtSignature(params, e.config["secret"])
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("epusdt encode quote request: %w", err)
	}
	var quote epusdtQuote
	if err := e.doJSON(ctx, http.MethodPost, gmPayQuotePath, body, &quote); err != nil {
		return nil, fmt.Errorf("epusdt quote USDT: %w", err)
	}
	rate, quotedAmount, err := validateEpusdtQuote(quote, amount, network)
	if err != nil {
		return nil, err
	}
	return &payment.USDTQuoteResponse{
		Currency:        "CNY",
		Token:           "USDT",
		Network:         network,
		Amount:          amount.InexactFloat64(),
		Rate:            rate.InexactFloat64(),
		QuotedAmount:    quotedAmount.InexactFloat64(),
		AmountPrecision: quote.AmountPrecision,
		RateSource:      quote.RateSource,
		RateQuoteAt:     quote.RateQuoteAt,
	}, nil
}

func (e *Epusdt) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	network, err := epusdtNetworkForPaymentType(req.PaymentType)
	if err != nil {
		return nil, err
	}
	amount, err := epusdtAmount(req.Amount)
	if err != nil {
		return nil, err
	}
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(e.config["notifyUrl"])
	}
	params := map[string]any{
		"pid":          strings.TrimSpace(e.config["pid"]),
		"order_id":     strings.TrimSpace(req.OrderID),
		"currency":     "cny",
		"token":        "usdt",
		"network":      network,
		"amount":       json.Number(amount.StringFixed(2)),
		"notify_url":   notifyURL,
		"redirect_url": strings.TrimSpace(req.ReturnURL),
		"name":         strings.TrimSpace(req.Subject),
		"rate_policy":  epusdtRatePolicyLive,
	}
	params["signature"] = epusdtSignature(params, e.config["secret"])

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("epusdt encode create request: %w", err)
	}
	var order epusdtOrder
	if err := e.doJSON(ctx, http.MethodPost, gmPayCreatePath, body, &order); err != nil {
		return nil, fmt.Errorf("epusdt create payment: %w", err)
	}
	if strings.TrimSpace(order.TradeID) == "" || strings.TrimSpace(order.PaymentURL) == "" {
		return nil, fmt.Errorf("epusdt create payment: response missing trade_id or payment_url")
	}
	if order.OrderID != "" && order.OrderID != req.OrderID {
		return nil, fmt.Errorf("epusdt create payment: order_id mismatch")
	}
	if err := validateEpusdtPaymentURL(order.PaymentURL); err != nil {
		return nil, err
	}
	if err := validateEpusdtReceiveAddress(network, order.ReceiveAddress); err != nil {
		return nil, err
	}
	if err := validateEpusdtOrderAmounts(order, amount); err != nil {
		return nil, err
	}
	if err := validateEpusdtLiveQuote(order, amount); err != nil {
		return nil, err
	}
	if order.Token != "" && !strings.EqualFold(order.Token, "usdt") {
		return nil, fmt.Errorf("epusdt create payment: unexpected token %s", order.Token)
	}
	if order.Network != "" && !strings.EqualFold(order.Network, network) {
		return nil, fmt.Errorf("epusdt create payment: unexpected network %s", order.Network)
	}
	return &payment.CreatePaymentResponse{
		TradeNo:            order.TradeID,
		PayURL:             order.PaymentURL,
		QRCode:             strings.TrimSpace(order.ReceiveAddress),
		PaymentAddress:     strings.TrimSpace(order.ReceiveAddress),
		PaymentTokenAmount: order.ActualAmount.String(),
		PaymentToken:       "USDT",
		PaymentNetwork:     network,
		Currency:           payment.DefaultPaymentCurrency,
	}, nil
}

func epusdtNetworkForPaymentType(paymentType string) (string, error) {
	switch strings.TrimSpace(paymentType) {
	case payment.TypeUSDTTron:
		return "tron", nil
	case payment.TypeUSDTBEP20:
		return "binance", nil
	default:
		return "", fmt.Errorf("epusdt unsupported payment type: %s", paymentType)
	}
}

func epusdtAmount(raw string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !amount.IsPositive() {
		return decimal.Zero, fmt.Errorf("epusdt invalid CNY amount")
	}
	if !amount.Equal(amount.Round(2)) {
		return decimal.Zero, fmt.Errorf("epusdt CNY amount must not have more than 2 decimal places")
	}
	return amount, nil
}

func validateEpusdtPaymentURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("epusdt create payment: invalid payment_url")
	}
	return nil
}

func validateEpusdtReceiveAddress(network, address string) error {
	address = strings.TrimSpace(address)
	switch network {
	case "tron":
		if len(address) != 34 || !strings.HasPrefix(address, "T") {
			return fmt.Errorf("epusdt create payment: invalid TRON receive address")
		}
	case "binance":
		if len(address) != 42 || !strings.HasPrefix(address, "0x") {
			return fmt.Errorf("epusdt create payment: invalid BEP20 receive address")
		}
		if _, err := hex.DecodeString(address[2:]); err != nil {
			return fmt.Errorf("epusdt create payment: invalid BEP20 receive address")
		}
	default:
		return fmt.Errorf("epusdt create payment: unexpected network %s", network)
	}
	return nil
}

func validateEpusdtOrderAmounts(order epusdtOrder, expected decimal.Decimal) error {
	amount, err := decimal.NewFromString(order.Amount.String())
	if err != nil || !amount.Equal(expected) {
		return fmt.Errorf("epusdt create payment: CNY amount mismatch")
	}
	actual, err := decimal.NewFromString(order.ActualAmount.String())
	if err != nil || !actual.IsPositive() {
		return fmt.Errorf("epusdt create payment: invalid locked USDT amount")
	}
	return nil
}

func validateEpusdtLiveQuote(order epusdtOrder, cnyAmount decimal.Decimal) error {
	if strings.TrimSpace(order.RatePolicy) != epusdtRatePolicyLive {
		return fmt.Errorf("epusdt create payment: live rate policy was not honored")
	}
	if order.AmountPrecision != 6 {
		return fmt.Errorf("epusdt create payment: live USDT amount precision must be 6")
	}
	lockedRate, err := decimal.NewFromString(order.LockedRate.String())
	if err != nil || !lockedRate.IsPositive() {
		return fmt.Errorf("epusdt create payment: invalid locked rate")
	}
	quotedAmount, err := decimal.NewFromString(order.QuotedAmount.String())
	if err != nil || !quotedAmount.IsPositive() {
		return fmt.Errorf("epusdt create payment: invalid quoted USDT amount")
	}
	expectedQuote := cnyAmount.Mul(lockedRate).Round(int32(order.AmountPrecision))
	if !quotedAmount.Equal(expectedQuote) {
		return fmt.Errorf("epusdt create payment: quoted USDT amount mismatch")
	}
	actualAmount, err := decimal.NewFromString(order.ActualAmount.String())
	if err != nil || actualAmount.LessThan(quotedAmount) {
		return fmt.Errorf("epusdt create payment: actual USDT amount is below quote")
	}
	maxReservationAdjustment := decimal.New(100, int32(-order.AmountPrecision))
	if actualAmount.Sub(quotedAmount).GreaterThan(maxReservationAdjustment) {
		return fmt.Errorf("epusdt create payment: actual USDT amount exceeds reservation adjustment")
	}
	sourceURL, err := url.Parse(strings.TrimSpace(order.RateSource))
	if err != nil || sourceURL.Scheme != "https" || sourceURL.Host == "" {
		return fmt.Errorf("epusdt create payment: invalid live rate source")
	}
	if order.RateQuoteAt <= 0 {
		return fmt.Errorf("epusdt create payment: invalid live rate timestamp")
	}
	age := time.Since(time.Unix(order.RateQuoteAt, 0))
	if age < -epusdtMaxFutureSkew || age > epusdtMaxLiveQuoteAge {
		return fmt.Errorf("epusdt create payment: live rate timestamp is stale")
	}
	return nil
}

func validateEpusdtQuote(quote epusdtQuote, expectedAmount decimal.Decimal, expectedNetwork string) (decimal.Decimal, decimal.Decimal, error) {
	if !strings.EqualFold(strings.TrimSpace(quote.Currency), "CNY") ||
		!strings.EqualFold(strings.TrimSpace(quote.Token), "USDT") ||
		!strings.EqualFold(strings.TrimSpace(quote.Network), expectedNetwork) {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: quote identity mismatch")
	}
	if strings.TrimSpace(quote.RatePolicy) != epusdtRatePolicyLive || quote.AmountPrecision != 6 {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: live rate policy was not honored")
	}
	amount, err := decimal.NewFromString(quote.Amount.String())
	if err != nil || !amount.Equal(expectedAmount) {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: CNY amount mismatch")
	}
	rate, err := decimal.NewFromString(quote.Rate.String())
	if err != nil || !rate.IsPositive() {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: invalid rate")
	}
	quotedAmount, err := decimal.NewFromString(quote.QuotedAmount.String())
	if err != nil || !quotedAmount.Equal(expectedAmount.Mul(rate).Round(int32(quote.AmountPrecision))) {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: quoted amount mismatch")
	}
	sourceURL, err := url.Parse(strings.TrimSpace(quote.RateSource))
	if err != nil || sourceURL.Scheme != "https" || sourceURL.Host == "" {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: invalid live rate source")
	}
	age := time.Since(time.Unix(quote.RateQuoteAt, 0))
	if quote.RateQuoteAt <= 0 || age < -epusdtMaxFutureSkew || age > epusdtMaxLiveQuoteAge {
		return decimal.Zero, decimal.Zero, fmt.Errorf("epusdt quote USDT: live rate timestamp is stale")
	}
	return rate, quotedAmount, nil
}

func (e *Epusdt) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("epusdt query requires trade_id")
	}
	var order epusdtOrder
	if err := e.doJSON(ctx, http.MethodGet, gmPayQueryPath+url.PathEscape(tradeNo), nil, &order); err != nil {
		return nil, fmt.Errorf("epusdt query order: %w", err)
	}
	if strings.TrimSpace(order.TradeID) == "" || order.TradeID != tradeNo {
		return nil, fmt.Errorf("epusdt query order: trade_id mismatch")
	}
	statusCode, err := strconv.Atoi(order.Status.String())
	if err != nil {
		return nil, fmt.Errorf("epusdt query order: invalid status")
	}
	status := payment.ProviderStatusPending
	if statusCode == epusdtStatusPaid {
		status = payment.ProviderStatusPaid
	} else if statusCode == epusdtStatusExpired {
		status = payment.ProviderStatusFailed
	}
	amount, err := decimal.NewFromString(order.Amount.String())
	if err != nil || !amount.IsPositive() {
		return nil, fmt.Errorf("epusdt query order: invalid CNY amount")
	}
	metadata := e.MerchantIdentityMetadata()
	metadata["currency"] = payment.DefaultPaymentCurrency
	metadata["token"] = strings.ToUpper(strings.TrimSpace(order.Token))
	metadata["network"] = strings.ToLower(strings.TrimSpace(order.Network))
	if metadata["token"] != "USDT" {
		return nil, fmt.Errorf("epusdt query order: unexpected token")
	}
	if metadata["network"] != "tron" && metadata["network"] != "binance" {
		return nil, fmt.Errorf("epusdt query order: unexpected network")
	}
	metadata["status"] = order.Status.String()
	return &payment.QueryOrderResponse{
		TradeNo:  order.TradeID,
		Status:   status,
		Amount:   amount.InexactFloat64(),
		Metadata: metadata,
	}, nil
}

func (e *Epusdt) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(rawBody))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("epusdt parse notification: %w", err)
	}
	signature := strings.ToLower(strings.TrimSpace(epusdtString(payload["signature"])))
	if signature == "" || !hmac.Equal([]byte(signature), []byte(epusdtSignature(epusdtNotificationSignParams(payload), e.config["secret"]))) {
		return nil, fmt.Errorf("epusdt invalid notification signature")
	}
	pid := strings.TrimSpace(epusdtString(payload["pid"]))
	if pid == "" || pid != strings.TrimSpace(e.config["pid"]) {
		return nil, fmt.Errorf("epusdt notification pid mismatch")
	}
	tradeID := strings.TrimSpace(epusdtString(payload["trade_id"]))
	orderID := strings.TrimSpace(epusdtString(payload["order_id"]))
	if tradeID == "" || orderID == "" {
		return nil, fmt.Errorf("epusdt notification missing order identifier")
	}
	amount, err := decimal.NewFromString(epusdtString(payload["amount"]))
	if err != nil || !amount.IsPositive() {
		return nil, fmt.Errorf("epusdt notification invalid CNY amount")
	}
	statusCode, err := strconv.Atoi(epusdtString(payload["status"]))
	if err != nil {
		return nil, fmt.Errorf("epusdt notification invalid status")
	}
	status := payment.ProviderStatusFailed
	if statusCode == epusdtStatusPaid {
		actualAmount, actualErr := decimal.NewFromString(epusdtString(payload["actual_amount"]))
		if actualErr != nil || !actualAmount.IsPositive() {
			return nil, fmt.Errorf("epusdt notification invalid paid USDT amount")
		}
		status = payment.ProviderStatusSuccess
	}
	metadata := e.MerchantIdentityMetadata()
	metadata["currency"] = payment.DefaultPaymentCurrency
	metadata["token"] = strings.ToUpper(strings.TrimSpace(epusdtString(payload["token"])))
	metadata["network"] = strings.ToLower(strings.TrimSpace(epusdtString(payload["network"])))
	if metadata["token"] != "USDT" {
		return nil, fmt.Errorf("epusdt notification unexpected token")
	}
	if metadata["network"] != "tron" && metadata["network"] != "binance" {
		return nil, fmt.Errorf("epusdt notification unexpected network")
	}
	metadata["status"] = strconv.Itoa(statusCode)
	return &payment.PaymentNotification{
		TradeNo:  tradeID,
		OrderID:  orderID,
		Amount:   amount.InexactFloat64(),
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func epusdtNotificationSignParams(payload map[string]any) map[string]any {
	params := make(map[string]any, 9)
	for _, key := range []string{"pid", "trade_id", "order_id", "amount", "actual_amount", "receive_address", "token", "network", "block_transaction_id", "status"} {
		if value, ok := payload[key]; ok {
			params[key] = value
		}
	}
	return params
}

func epusdtSignature(params map[string]any, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "signature" || value == nil || epusdtString(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+epusdtString(params[key]))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(mac.Sum(nil))
}

func epusdtString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		if normalized, err := decimal.NewFromString(typed.String()); err == nil {
			return normalized.String()
		}
		return typed.String()
	case decimal.Decimal:
		return typed.String()
	case float64:
		return decimal.NewFromFloat(typed).String()
	case float32:
		return decimal.NewFromFloat32(typed).String()
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func (e *Epusdt) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("epusdt on-chain refunds are disabled")
}

func (e *Epusdt) doJSON(ctx context.Context, method, path string, body []byte, target any) error {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.config["apiBase"]+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, epusdtMaxResponseSize))
	if err != nil {
		return err
	}
	var envelope epusdtAPIResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	statusCode, _ := strconv.Atoi(envelope.StatusCode.String())
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || statusCode != http.StatusOK {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("gateway HTTP %d: %s", resp.StatusCode, message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("gateway returned empty data")
	}
	decoder = json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode response data: %w", err)
	}
	return nil
}

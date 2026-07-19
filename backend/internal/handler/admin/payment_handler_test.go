package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
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

func TestAdminListOrdersResponseDerivesCurrencyAndOmitsProviderSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	client := newAdminPaymentHandlerTestClient(t)

	user, err := client.User.Create().
		SetEmail("admin-list-orders@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-list-orders-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(101).
		SetRechargeCode("LIST-JPY").
		SetOutTradeNo("sub2_admin_list_jpy").
		SetPaymentType(payment.TypeAirwallex).
		SetPaymentTradeNo("trade-list-jpy").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderSnapshot(map[string]any{"currency": "jpy", "merchant_id": "secret_list_merchant"}).
		Save(ctx)
	require.NoError(t, err)

	router := gin.New()
	handler := NewPaymentHandler(service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, nil, nil, nil), nil)
	router.GET("/api/v1/admin/payment/orders", handler.ListOrders)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/orders?page=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"currency":"JPY"`)
	require.NotContains(t, body, "provider_snapshot")
	require.NotContains(t, body, "secret_list_merchant")

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				ID       int64  `json:"id"`
				Currency string `json:"currency"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, int64(1), envelope.Data.Total)
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "JPY", envelope.Data.Items[0].Currency)
}

func TestAdminGetOrderDetailResponseDerivesCurrencyAndOmitsProviderSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	client := newAdminPaymentHandlerTestClient(t)

	user, err := client.User.Create().
		SetEmail("admin-order-detail@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-order-detail-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(20).
		SetPayAmount(22).
		SetRechargeCode("DETAIL-HKD").
		SetOutTradeNo("sub2_admin_detail_hkd").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade-detail-hkd").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderSnapshot(map[string]any{"currency": "hkd", "merchant_id": "secret_detail_merchant"}).
		Save(ctx)
	require.NoError(t, err)

	router := gin.New()
	handler := NewPaymentHandler(service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, nil, nil, nil), nil)
	router.GET("/api/v1/admin/payment/orders/:id", handler.GetOrderDetail)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/admin/payment/orders/%d", order.ID), nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"currency":"HKD"`)
	require.NotContains(t, body, "provider_snapshot")
	require.NotContains(t, body, "secret_detail_merchant")

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Order struct {
				ID       int64  `json:"id"`
				Currency string `json:"currency"`
			} `json:"order"`
			AuditLogs []any `json:"auditLogs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, order.ID, envelope.Data.Order.ID)
	require.Equal(t, "HKD", envelope.Data.Order.Currency)
	require.Empty(t, envelope.Data.AuditLogs)
}

func newAdminPaymentHandlerTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

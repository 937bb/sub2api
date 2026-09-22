package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAICodexInfrastructureCookieStoreFiltersAndExpiresCookies(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	store := newOpenAICodexInfrastructureCookieStore()
	headers := http.Header{}
	headers.Add("Set-Cookie", "__cflb=route-a; Max-Age=60; Path=/; Secure; HttpOnly")
	headers.Add("Set-Cookie", "cf_chl_rc_i=challenge-a; Path=/; Secure")
	headers.Add("Set-Cookie", "session_token=must-not-store; Path=/; Secure; HttpOnly")
	headers.Add("Set-Cookie", "__oailb=already-expired; Expires=Mon, 21 Sep 2026 12:00:00 GMT; Path=/")

	store.capture(headers, now)

	require.Equal(t, "__cflb=route-a; cf_chl_rc_i=challenge-a", store.header(now))
	require.NotContains(t, store.header(now), "session_token")
	require.Equal(t, "cf_chl_rc_i=challenge-a", store.header(now.Add(61*time.Second)))
}

func TestOpenAICodexInfrastructureCookiesShareHTTPStateWithWSAndStripPrivateCookies(t *testing.T) {
	svc := &OpenAIGatewayService{}
	httpResponseHeaders := http.Header{}
	httpResponseHeaders.Add("Set-Cookie", "__cf_bm=http-route; Path=/; Secure; HttpOnly")
	httpResponseHeaders.Add("Set-Cookie", "auth_session=must-not-store; Path=/; Secure; HttpOnly")
	svc.captureOpenAICodexInfrastructureCookies(httpResponseHeaders)

	wsHeaders := http.Header{"Cookie": []string{"downstream_session=must-not-forward"}}
	svc.applyOpenAICodexInfrastructureCookies(wsHeaders)

	require.Equal(t, "__cf_bm=http-route", wsHeaders.Get("Cookie"))
	require.NotContains(t, wsHeaders.Get("Cookie"), "auth_session")
	require.NotContains(t, wsHeaders.Get("Cookie"), "downstream_session")
}

func TestOpenAICodexInfrastructureCookiesRouteTicketOverridesSharedValues(t *testing.T) {
	svc := &OpenAIGatewayService{}
	sharedHeaders := http.Header{}
	sharedHeaders.Add("Set-Cookie", "__cflb=shared-route; Path=/")
	sharedHeaders.Add("Set-Cookie", "__cf_bm=shared-bot-cookie; Path=/")
	svc.captureOpenAICodexInfrastructureCookies(sharedHeaders)

	headers := http.Header{
		"Cookie": []string{"private=value; __cflb=ticket-route; __oailb=ticket-openai-route"},
	}
	svc.applyOpenAICodexInfrastructureCookies(headers)

	require.Equal(t, "__cflb=ticket-route; __oailb=ticket-openai-route; __cf_bm=shared-bot-cookie", headers.Get("Cookie"))
	require.NotContains(t, headers.Get("Cookie"), "private")
}

func TestOpenAICodexInfrastructureCookiesCaptureFailedWSHandshake(t *testing.T) {
	svc := &OpenAIGatewayService{}
	responseHeaders := http.Header{}
	responseHeaders.Add("Set-Cookie", "_cfuvid=ws-route; Path=/; Secure")
	err := &openAIWSDialError{ResponseHeaders: responseHeaders, Err: errors.New("handshake rejected")}

	svc.captureOpenAICodexInfrastructureCookiesFromWSError(err)
	headers := http.Header{}
	svc.applyOpenAICodexInfrastructureCookies(headers)

	require.Equal(t, "_cfuvid=ws-route", headers.Get("Cookie"))
}

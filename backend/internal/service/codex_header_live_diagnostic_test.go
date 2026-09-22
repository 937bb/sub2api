//go:build codexdiagnostic

package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptrelay"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/tidwall/gjson"
)

// TestCodexHeaderLiveDiagnostic is an explicitly enabled operational experiment.
// Supply authorized account snapshots through stdin, never command arguments.
// It uses production header builders and the production WS dialer, but no
// scheduler, repository writes, retries, or production connection pools.
func TestCodexHeaderLiveDiagnostic(t *testing.T) {
	if os.Getenv("CODEX_HEADER_DIAGNOSTIC") != "1" {
		t.Skip("live diagnostic requires explicit enablement and account snapshots")
	}
	var input struct {
		Accounts         []*Account
		Models           []string
		Transports       []string
		Variants         []string
		Version          string
		Relay            string
		SourceIPv6       string
		SourceIPv6s      []string
		Rounds           int
		DefaultFull      bool
		InspectModels    bool
		FreshConnections bool
		FreshSessions    bool
		CookieMode       string
		SharedCookieJar  bool
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		t.Fatal("invalid diagnostic input")
	}
	if len(input.Accounts) == 0 || len(input.Accounts) > 3 || len(input.Models) == 0 || len(input.Models) > 2 || input.Rounds < 1 || input.Rounds > 8 || NormalizeCodexClientVersion(input.Version) == "" || len(input.SourceIPv6s) > 16 {
		t.Fatal("invalid diagnostic bounds")
	}
	if len(input.SourceIPv6s) > 0 && strings.TrimSpace(input.Relay) == "" {
		t.Fatal("source IPv6 rotation requires the relay")
	}
	for _, source := range input.SourceIPv6s {
		address := net.ParseIP(strings.TrimSpace(source))
		if address == nil || address.To4() != nil {
			t.Fatal("invalid rotating source IPv6")
		}
	}
	if len(input.Transports) == 0 || len(input.Transports) > 2 {
		t.Fatal("choose HTTP and/or WS")
	}
	if input.CookieMode == "" {
		input.CookieMode = "off"
	}
	if input.CookieMode != "off" && input.CookieMode != "affinity" && input.CookieMode != "all" {
		t.Fatal("unsupported cookie mode")
	}
	for _, transport := range input.Transports {
		if transport != "http" && transport != "ws" {
			t.Fatal("unsupported transport")
		}
	}
	if len(input.Variants) == 0 {
		input.Variants = []string{"current", "mac_current", "mac_1533", "cli_rs", "minimal_features"}
	}
	if len(input.Variants) > 5 {
		t.Fatal("too many variants")
	}
	for _, variant := range input.Variants {
		switch variant {
		case "current", "mac_current", "mac_1533", "cli_rs", "minimal_features", "legacy_http_beta",
			"no_routing_hint", "official_headers", "ws_minimal_frame", "named_stream", "ws_no_compression", "http_zstd", "http_h1", "device_mode", "lite_compatible":
		default:
			t.Fatal("unsupported header variant")
		}
	}
	SetCodexCanonicalUserAgentResolver(func() string { return buildCodexCLIUserAgent(input.Version) })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	cfg := &config.Config{}
	cfg.Gateway.OpenAIChatGPTIPv6Only = input.Relay != ""
	cfg.Gateway.OpenAIChatGPTIPv6RelayAddr = input.Relay
	svc := &OpenAIGatewayService{cfg: cfg}
	dialer := newConfiguredOpenAIWSClientDialer(cfg)
	var fixedSourceTransport *http.Transport
	if input.SourceIPv6 != "" {
		address := net.ParseIP(input.SourceIPv6)
		if address == nil || address.To4() != nil {
			t.Fatal("fixed source must be an assigned IPv6 address")
		}
		netDialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, LocalAddr: &net.TCPAddr{IP: address}}
		fixedSourceTransport = &http.Transport{TLSHandshakeTimeout: 10 * time.Second,
			DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
				return netDialer.DialContext(ctx, "tcp6", address)
			}}
		t.Cleanup(fixedSourceTransport.CloseIdleConnections)
		dialer = &coderOpenAIWSClientDialer{directClient: &http.Client{Transport: fixedSourceTransport},
			proxyClients: make(map[string]*openAIWSProxyClientEntry), upstreamReadLimitBytes: openAIWSUpstreamReadLimitBytesDefault}
	}
	connections := make(map[string]openAIWSClientConn)
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	clients := make(map[int64]*http.Client)
	http1Clients := make(map[int64]*http.Client)
	httpTransports := make(map[int64]*http.Transport)
	http1Transports := make(map[int64]*http.Transport)
	sessions := make(map[int64]string)
	retryAfter := make(map[int64]time.Time)
	var sharedJar http.CookieJar
	if input.SharedCookieJar {
		sharedJar = codexDiagnosticCookieJar(t, input.CookieMode)
	}
	sourceIndex := 0
	for _, account := range input.Accounts {
		if account == nil || !account.IsOpenAIOAuthLike() || account.GetCredential("access_token") == "" || account.ProxyID != nil {
			t.Fatal("diagnostic requires direct OAuth/PAT account snapshots")
		}
		netDialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		transport := &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second,
			DialContext: chatgptrelay.Wrap(netDialer.DialContext, chatgptrelay.Settings{Enabled: input.Relay != "", RelayAddr: input.Relay})}
		if fixedSourceTransport != nil {
			transport.DialContext = fixedSourceTransport.DialContext
		}
		t.Cleanup(transport.CloseIdleConnections)
		jar := sharedJar
		if !input.SharedCookieJar {
			jar = codexDiagnosticCookieJar(t, input.CookieMode)
		}
		clients[account.ID] = &http.Client{Transport: transport, Jar: jar}
		httpTransports[account.ID] = transport
		http1Transport := codexDiagnosticHTTP1Transport(transport)
		t.Cleanup(http1Transport.CloseIdleConnections)
		http1Clients[account.ID] = &http.Client{Transport: http1Transport, Jar: jar}
		http1Transports[account.ID] = http1Transport
		sessions[account.ID] = uuid.NewString()
		if input.InspectModels {
			codexDiagnosticModels(t, clients[account.ID], svc, account, input.Version)
		}
	}
	for round := range input.Rounds {
		for _, model := range input.Models {
			for _, transport := range input.Transports {
				for _, sourceAccount := range input.Accounts {
					for index := range input.Variants {
						if round%2 == 1 {
							index = len(input.Variants) - 1 - index
						}
						variant := input.Variants[index]
						if remaining := time.Until(retryAfter[sourceAccount.ID]); remaining > 0 {
							encoded, _ := json.Marshal(map[string]any{"round": round + 1, "account_id": sourceAccount.ID,
								"model": model, "transport": transport, "variant": variant, "skipped": true,
								"reason": "upstream_retry_after", "remaining_ms": remaining.Milliseconds()})
							fmt.Println(string(encoded))
							continue
						}
						account := sourceAccount
						if variant == "device_mode" {
							if _, valid := codexFingerprintSeed(sourceAccount.Extra); !valid {
								t.Fatal("device comparison requires an existing account seed")
							}
							snapshot := *sourceAccount
							snapshot.Extra = maps.Clone(sourceAccount.Extra)
							snapshot.Extra[codexFingerprintModeExtraKey] = string(codexFingerprintDevice)
							account = &snapshot
						}
						ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
						sourceIPv6 := strings.TrimSpace(input.SourceIPv6)
						if len(input.SourceIPv6s) > 0 {
							sourceIPv6 = strings.TrimSpace(input.SourceIPv6s[sourceIndex%len(input.SourceIPv6s)])
							sourceIndex++
							ctx = chatgptrelay.WithSourceIPv6(ctx, sourceIPv6)
						}
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						session := sessions[account.ID]
						if input.FreshSessions {
							session = uuid.NewString()
						}
						c.Request.Header.Set("session-id", session)
						c.Request.Header.Set("thread-id", session)
						payload := createOpenAITestPayload(model, true)
						payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "Reply exactly OK."}}}}
						payload["prompt_cache_key"] = session
						payload["client_metadata"] = map[string]any{"session_id": session, "thread_id": session}
						if variant == "lite_compatible" {
							codexDiagnosticLitePayload(t, payload, transport)
						}
						applyCodexClientMetadata(payload, account)
						applyCodexAccountIdentityClientMetadataMap(payload, account, 0)
						ids := resolveCodexFingerprintIDsFromRequestWithDefault(account, c.Request.Header, input.DefaultFull)
						applyCodexFingerprintClientMetadata(payload, ids)
						stageCodexFingerprintIDs(c, ids)
						body, err := json.Marshal(payload)
						if err != nil {
							t.Fatal("could not encode diagnostic payload")
						}
						req, err := svc.buildUpstreamRequest(ctx, c, account, body, account.GetCredential("access_token"), true, session, true)
						if err != nil {
							t.Fatal("could not build diagnostic request")
						}
						if transport == "ws" {
							req.Header, _, err = svc.buildOpenAIWSHeaders(ctx, c, account, account.GetCredential("access_token"), OpenAIWSProtocolDecision{}, true, "", "", session, model, "")
							if err != nil {
								t.Fatal("could not build WS headers")
							}
						}
						applyCodexNormalizedRequestIdentityHeaders(c, account, req.Header, body)
						applyStagedCodexFingerprintHeaders(c, account, req.Header)
						codexDiagnosticVariant(req.Header, variant, input.Version)
						if transport == "ws" {
							codexDiagnosticApplyJarCookies(req, clients[account.ID].Jar)
						}
						if variant == "lite_compatible" && transport == "http" {
							req.Header.Set(responsesLiteHeader, "true")
						}
						if variant == "http_zstd" {
							encoder, encodeErr := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
							if encodeErr != nil {
								t.Fatal("could not initialize diagnostic compression")
							}
							compressed := encoder.EncodeAll(body, nil)
							_ = encoder.Close()
							req.Body = io.NopCloser(bytes.NewReader(compressed))
							req.ContentLength = int64(len(compressed))
							req.Header.Set("Content-Encoding", "zstd")
						}
						observation := map[string]any{"round": round + 1, "account_id": account.ID, "model": model, "transport": transport, "variant": variant,
							"user_agent": req.Header.Get("User-Agent"), "version": req.Header.Get("version"), "originator": req.Header.Get("originator"),
							"beta": req.Header.Get("OpenAI-Beta"), "features": req.Header.Get("x-codex-beta-features"), "success": false}
						observation["fixed_egress"] = sourceIPv6 != ""
						observation["source_ipv6"] = sourceIPv6
						observation["fresh_connection_policy"] = input.FreshConnections
						observation["cookie_mode"] = input.CookieMode
						if client := clients[account.ID]; client != nil && client.Jar != nil {
							observation["sent_cookies"] = codexDiagnosticCookieSummary(client.Jar.Cookies(req.URL))
						}
						observation["routing_hint"] = req.Header.Get(openAICodexRoutingHintHeader)
						started := time.Now()
						if transport == "http" {
							var response *http.Response
							client := clients[account.ID]
							if variant == "http_h1" {
								client = http1Clients[account.ID]
							}
							response, err = client.Do(req)
							if err == nil {
								observation["http_status"] = response.StatusCode
								observation["http_protocol"] = response.Proto
								observation["set_cookies"] = codexDiagnosticCookieSummary(response.Cookies())
								observation["cf_ray"] = response.Header.Get("Cf-Ray")
								observation["proxy_wasm"] = response.Header.Get("X-Openai-Proxy-Wasm")
								codexDiagnosticRoutingHeaders(response.Header, observation)
								if response.StatusCode != http.StatusOK {
									raw, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
									codexDiagnosticHTTPError(response.Header, raw, observation)
								} else {
									scanner := bufio.NewScanner(response.Body)
									scanner.Buffer(make([]byte, 4096), 2<<20)
									for scanner.Scan() {
										if raw, ok := bytes.CutPrefix(scanner.Bytes(), []byte("data:")); ok && codexDiagnosticEvent(raw, started, observation) {
											break
										}
									}
									err = scanner.Err()
								}
								_ = response.Body.Close()
								if input.FreshConnections {
									if variant == "http_h1" {
										http1Transports[account.ID].CloseIdleConnections()
									} else {
										httpTransports[account.ID].CloseIdleConnections()
									}
								}
							}
						} else {
							key := fmt.Sprintf("%d/%s/%s", account.ID, model, variant)
							conn := connections[key]
							observation["connection_reused"] = conn != nil
							if conn == nil {
								var status int
								var responseHeaders http.Header
								conn, status, responseHeaders, err = codexDiagnosticDial(ctx, dialer, variant, strings.Replace(req.URL.String(), "https://", "wss://", 1), req.Header)
								observation["handshake_status"] = status
								observation["ws_extensions"] = responseHeaders.Get("Sec-WebSocket-Extensions")
								var handshakeError *openAIWSHandshakeError
								if errors.As(err, &handshakeError) {
									codexDiagnosticHTTPError(responseHeaders, handshakeError.Body, observation)
								}
								if err == nil {
									connections[key] = conn
									observation["handshake_status"] = http.StatusSwitchingProtocols
								}
							}
							if err == nil {
								payload["type"] = "response.create"
								if variant == "ws_minimal_frame" || variant == "named_stream" {
									delete(payload, "stream")
									delete(payload, "background")
								}
								if variant == "named_stream" {
									payload["stream_id"] = "diagnostic_main"
									observation["stream_id_echoed"] = false
								}
								err = conn.WriteJSON(ctx, payload)
							}
							for err == nil {
								var raw []byte
								raw, err = conn.ReadMessage(ctx)
								if variant == "named_stream" && gjson.GetBytes(raw, "stream_id").String() == "diagnostic_main" {
									observation["stream_id_echoed"] = true
								}
								if err == nil && codexDiagnosticEvent(raw, started, observation) {
									break
								}
							}
							// An error event can precede response.failed. Retire this
							// diagnostic socket so that tail cannot be attributed to
							// the next request. Only completed responses are reusable.
							if (input.FreshConnections || err != nil || observation["success"] != true) && conn != nil {
								_ = conn.Close()
								delete(connections, key)
							}
						}
						if err != nil {
							observation["transport_error"] = strings.ReplaceAll(err.Error(), account.GetCredential("access_token"), "[REDACTED]")
						}
						observation["duration_ms"] = time.Since(started).Milliseconds()
						if value, ok := observation["retry_after"].(string); ok && value != "" {
							if seconds, parseErr := strconv.Atoi(value); parseErr == nil && seconds > 0 {
								retryAfter[account.ID] = time.Now().Add(time.Duration(seconds) * time.Second)
							} else if deadline, parseErr := http.ParseTime(value); parseErr == nil {
								retryAfter[account.ID] = deadline
							}
						}
						cancel()
						encoded, _ := json.Marshal(observation)
						fmt.Println(strings.ReplaceAll(string(encoded), account.GetCredential("access_token"), "[REDACTED]"))
					}
				}
			}
		}
	}
}

type codexRouteAffinityDiagnosticResult struct {
	state       string
	routeCookie string
	observation map[string]any
}

// TestCodexRouteAffinityLiveDiagnostic compares State, affinity Cookie, and
// session identity on fresh HTTP connections. It is operational-only and never
// prints raw credentials, State, or Cookie values.
func TestCodexRouteAffinityLiveDiagnostic(t *testing.T) {
	if os.Getenv("CODEX_ROUTE_AFFINITY_DIAGNOSTIC") != "1" {
		t.Skip("live route-affinity diagnostic requires explicit enablement")
	}
	var input struct {
		Accounts    []*Account
		Model       string
		Version     string
		Relay       string
		SourceIPv6s []string
		WaitSeconds int
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		t.Fatal("invalid route-affinity diagnostic input")
	}
	if len(input.Accounts) != 1 || input.Accounts[0] == nil || !input.Accounts[0].IsOpenAIOAuthLike() ||
		input.Accounts[0].GetCredential("access_token") == "" || input.Accounts[0].ProxyID != nil ||
		normalizeOpenAICodexTurnStateModel(input.Model) == "" || NormalizeCodexClientVersion(input.Version) == "" ||
		input.WaitSeconds < 0 || input.WaitSeconds > 300 || len(input.SourceIPv6s) > 8 {
		t.Fatal("invalid route-affinity diagnostic bounds")
	}
	if len(input.SourceIPv6s) > 0 && strings.TrimSpace(input.Relay) == "" {
		t.Fatal("source IPv6 rotation requires the relay")
	}
	for _, source := range input.SourceIPv6s {
		address := net.ParseIP(strings.TrimSpace(source))
		if address == nil || address.To4() != nil {
			t.Fatal("invalid diagnostic source IPv6")
		}
	}
	SetCodexCanonicalUserAgentResolver(func() string { return buildCodexCLIUserAgent(input.Version) })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	cfg := &config.Config{}
	cfg.Gateway.OpenAIChatGPTIPv6Only = strings.TrimSpace(input.Relay) != ""
	cfg.Gateway.OpenAIChatGPTIPv6RelayAddr = strings.TrimSpace(input.Relay)
	svc := &OpenAIGatewayService{cfg: cfg}
	account := input.Accounts[0]
	baseSession := uuid.NewString()
	sourceIndex := 0
	nextSource := func() string {
		if len(input.SourceIPv6s) == 0 {
			return ""
		}
		source := input.SourceIPv6s[sourceIndex%len(input.SourceIPv6s)]
		sourceIndex++
		return source
	}

	seed := runCodexRouteAffinityDiagnosticRequest(t, svc, account, input.Model, baseSession, "", "", nextSource(), input.Relay, "seed")
	printCodexRouteAffinityDiagnostic(account, seed.observation)
	if seed.state == "" {
		t.Fatal("seed response did not return x-codex-turn-state")
	}
	variants := []struct {
		name        string
		state       string
		cookie      string
		sameSession bool
	}{
		{name: "state_cookie_session", state: seed.state, cookie: seed.routeCookie, sameSession: true},
		{name: "state_session", state: seed.state, sameSession: true},
		{name: "cookie_session", cookie: seed.routeCookie, sameSession: true},
		{name: "session_only", sameSession: true},
		{name: "state_cookie_new_session", state: seed.state, cookie: seed.routeCookie},
		{name: "cookie_new_session", cookie: seed.routeCookie},
		{name: "neither_new_session"},
	}
	for _, variant := range variants {
		sessionID := uuid.NewString()
		if variant.sameSession {
			sessionID = baseSession
		}
		result := runCodexRouteAffinityDiagnosticRequest(t, svc, account, input.Model, sessionID, variant.state, variant.cookie, nextSource(), input.Relay, variant.name)
		printCodexRouteAffinityDiagnostic(account, result.observation)
	}
	if input.WaitSeconds == 0 {
		return
	}
	waiting, _ := json.Marshal(map[string]any{"phase": "waiting", "seconds": input.WaitSeconds})
	fmt.Println(string(waiting))
	timer := time.NewTimer(time.Duration(input.WaitSeconds) * time.Second)
	defer timer.Stop()
	<-timer.C
	for _, variant := range []struct {
		name   string
		state  string
		cookie string
	}{
		{name: "post_wait_state_cookie", state: seed.state, cookie: seed.routeCookie},
		{name: "post_wait_state", state: seed.state},
		{name: "post_wait_cookie", cookie: seed.routeCookie},
		{name: "post_wait_neither"},
	} {
		result := runCodexRouteAffinityDiagnosticRequest(t, svc, account, input.Model, baseSession, variant.state, variant.cookie, nextSource(), input.Relay, variant.name)
		printCodexRouteAffinityDiagnostic(account, result.observation)
	}
}

func runCodexRouteAffinityDiagnosticRequest(t *testing.T, svc *OpenAIGatewayService, account *Account, model, sessionID, state, routeCookie, sourceIPv6, relay, phase string) codexRouteAffinityDiagnosticResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if sourceIPv6 != "" {
		ctx = chatgptrelay.WithSourceIPv6(ctx, sourceIPv6)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", sessionID)
	c.Request.Header.Set("thread-id", sessionID)
	payload := createOpenAITestPayload(model, true)
	payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "Reply exactly OK."}}}}
	payload["prompt_cache_key"] = sessionID
	payload["client_metadata"] = map[string]any{"session_id": sessionID, "thread_id": sessionID}
	applyCodexClientMetadata(payload, account)
	applyCodexAccountIdentityClientMetadataMap(payload, account, 0)
	ids := resolveCodexFingerprintIDsFromRequestWithDefault(account, c.Request.Header, false)
	applyCodexFingerprintClientMetadata(payload, ids)
	stageCodexFingerprintIDs(c, ids)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal("could not encode route-affinity payload")
	}
	req, err := svc.buildUpstreamRequest(ctx, c, account, body, account.GetCredential("access_token"), true, sessionID, true)
	if err != nil {
		t.Fatal("could not build route-affinity request")
	}
	applyCodexNormalizedRequestIdentityHeaders(c, account, req.Header, body)
	applyStagedCodexFingerprintHeaders(c, account, req.Header)
	if state != "" {
		req.Header.Set(openAICodexTurnStateHeader, state)
	}
	if routeCookie = normalizeOpenAICodexAffinityCookieHeader(routeCookie); routeCookie != "" {
		req.Header.Set("Cookie", routeCookie)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: chatgptrelay.Wrap(dialer.DialContext, chatgptrelay.Settings{
			Enabled:   strings.TrimSpace(relay) != "",
			RelayAddr: strings.TrimSpace(relay),
		}),
	}
	defer transport.CloseIdleConnections()
	observation := map[string]any{
		"phase": phase, "account_id": account.ID, "requested_model": model,
		"sent_state": state != "", "sent_cookie": routeCookie != "", "source_ipv6": sourceIPv6,
		"session_hash": shortCodexDiagnosticHash(sessionID), "success": false,
	}
	started := time.Now()
	response, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		observation["transport_error"] = strings.ReplaceAll(err.Error(), account.GetCredential("access_token"), "[REDACTED]")
		observation["duration_ms"] = time.Since(started).Milliseconds()
		return codexRouteAffinityDiagnosticResult{observation: observation}
	}
	defer func() { _ = response.Body.Close() }()
	observation["http_status"] = response.StatusCode
	observation["http_protocol"] = response.Proto
	observation["official_model_header"] = firstNonEmptyCodexHeader(response.Header, "OpenAI-Model", "openai-model")
	observation["cf_ray"] = response.Header.Get("Cf-Ray")
	responseState := extractOpenAICodexTurnState(response.Header)
	if responseState != "" {
		observation["response_state_length"] = len(responseState)
		observation["response_state_hash"] = shortCodexDiagnosticHash(responseState)
	}
	responseCookie := mergeOpenAICodexAffinityCookies(routeCookie, response.Cookies())
	observation["set_cookies"] = codexDiagnosticCookieSummary(response.Cookies())
	observation["affinity_cookie_hash"] = shortCodexDiagnosticHash(responseCookie)
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		codexDiagnosticHTTPError(response.Header, raw, observation)
	} else {
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			if raw, ok := bytes.CutPrefix(scanner.Bytes(), []byte("data:")); ok && codexDiagnosticEvent(raw, started, observation) {
				break
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			observation["stream_error"] = scanErr.Error()
		}
	}
	observation["duration_ms"] = time.Since(started).Milliseconds()
	return codexRouteAffinityDiagnosticResult{state: responseState, routeCookie: responseCookie, observation: observation}
}

func printCodexRouteAffinityDiagnostic(account *Account, observation map[string]any) {
	encoded, _ := json.Marshal(observation)
	fmt.Println(strings.ReplaceAll(string(encoded), account.GetCredential("access_token"), "[REDACTED]"))
}

func shortCodexDiagnosticHash(value string) string {
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:6])
}

type codexDiagnosticFilteringJar struct {
	base    http.CookieJar
	allowed map[string]struct{}
}

func codexDiagnosticCookieJar(t *testing.T, mode string) http.CookieJar {
	t.Helper()
	if mode == "off" {
		return nil
	}
	base, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal("could not initialize diagnostic cookie jar")
	}
	if mode == "all" {
		return base
	}
	return &codexDiagnosticFilteringJar{base: base, allowed: map[string]struct{}{
		"__cf_bm":         {},
		"__cflb":          {},
		"__cfruid":        {},
		"__cfseq":         {},
		"__cfwaitingroom": {},
		"__oailb":         {},
		"_cfuvid":         {},
		"cf_clearance":    {},
		"cf_ob_info":      {},
		"cf_use_ob":       {},
	}}
}

func (j *codexDiagnosticFilteringJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	filtered := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if _, ok := j.allowed[cookie.Name]; ok || strings.HasPrefix(cookie.Name, "cf_chl_") {
			filtered = append(filtered, cookie)
		}
	}
	j.base.SetCookies(u, filtered)
}

func (j *codexDiagnosticFilteringJar) Cookies(u *url.URL) []*http.Cookie {
	return j.base.Cookies(u)
}

func codexDiagnosticApplyJarCookies(req *http.Request, jar http.CookieJar) {
	if req == nil || req.URL == nil || jar == nil {
		return
	}
	req.Header.Del("Cookie")
	for _, cookie := range jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
}

func codexDiagnosticCookieSummary(cookies []*http.Cookie) []string {
	summary := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		digest := sha256.Sum256([]byte(cookie.Value))
		summary = append(summary, fmt.Sprintf("%s:%x", cookie.Name, digest[:4]))
	}
	slices.Sort(summary)
	return summary
}

func codexDiagnosticRoutingHeaders(headers http.Header, observation map[string]any) {
	for _, name := range []string{
		"x-codex-active-limit",
		"x-codex-plan-type",
		"x-codex-primary-over-secondary-limit-percent",
		"x-codex-primary-used-percent",
		"x-codex-safety-buffering-enabled",
		"x-codex-safety-buffering-faster-model",
		"x-codex-secondary-used-percent",
	} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			observation[name] = value
		}
	}
	if state := strings.TrimSpace(headers.Get(openAICodexTurnStateHeader)); state != "" {
		digest := sha256.Sum256([]byte(state))
		observation["response_state_length"] = len(state)
		observation["response_state_hash"] = fmt.Sprintf("%x", digest[:6])
	}
}

func codexDiagnosticHTTP1Transport(base *http.Transport) *http.Transport {
	transport := base.Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	transport.ForceAttemptHTTP2 = false
	// Clone can retain an initialized HTTP/2 transport's ALPN advertisement.
	// Match the advertised wire protocol to the diagnostic response parser.
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	return transport
}

func TestCodexDiagnosticHTTP1TransportNegotiatesHTTP1(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Diagnostic-Protocol", r.Proto)
		w.WriteHeader(http.StatusNoContent)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	base, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("test server does not use an HTTP transport")
	}
	base.ForceAttemptHTTP2 = true
	check := func(client *http.Client, protocol string) {
		t.Helper()
		response, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.Proto != protocol || response.Header.Get("X-Diagnostic-Protocol") != protocol {
			t.Fatalf("protocol mismatch: client=%s server=%s want=%s", response.Proto, response.Header.Get("X-Diagnostic-Protocol"), protocol)
		}
	}
	check(server.Client(), "HTTP/2.0")
	http1 := codexDiagnosticHTTP1Transport(base)
	t.Cleanup(http1.CloseIdleConnections)
	check(&http.Client{Transport: http1}, "HTTP/1.1")
	base.CloseIdleConnections()
	check(server.Client(), "HTTP/2.0")
}

func codexDiagnosticLitePayload(t *testing.T, payload map[string]any, transport string) {
	t.Helper()
	// Mirror the official empty-tool Lite prefix and move the same trusted
	// instructions into an input message. User input is left unchanged.
	instructions, _ := payload["instructions"].(string)
	input, ok := payload["input"].([]map[string]any)
	if !ok {
		t.Fatal("unexpected diagnostic input")
	}
	prefix := []map[string]any{{"type": "additional_tools", "role": "developer", "tools": []any{}}}
	if instructions != "" {
		prefix = append(prefix, map[string]any{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": instructions}}})
	}
	payload["input"] = append(prefix, input...)
	payload["instructions"] = ""
	if _, err := normalizeOpenAIResponsesLiteTools(payload); err != nil {
		t.Fatal("could not normalize diagnostic Lite request")
	}
	if transport == "ws" {
		setOpenAIWSClientMetadata(payload, responsesLiteWSMetadataKey, "true")
	}
}

func codexDiagnosticVariant(headers http.Header, variant, version string) {
	var ua string
	switch variant {
	case "mac_current", "mac_1533":
		if variant == "mac_1533" {
			version = "0.153.3"
		}
		ua = "codex-tui/" + version + " (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; " + version + ")"
	case "cli_rs":
		ua = "codex_cli_rs/" + version + " (Mac OS 26.5.1; arm64) iTerm.app/3.6.11"
	case "minimal_features":
		headers.Del("x-codex-beta-features")
		stripOpenAILegacyResponsesBeta(headers)
	case "legacy_http_beta":
		if headers.Get("OpenAI-Beta") == "" {
			headers.Set("OpenAI-Beta", "responses=experimental")
		}
	case "no_routing_hint":
		headers.Del(openAICodexRoutingHintHeader)
	case "official_headers":
		// Current Codex compatibility headers use session_id and thread_id.
		// Retain the same identity values and supported WS negotiation token.
		for _, key := range []string{"version", "session-id", "thread-id", "conversation_id"} {
			headers.Del(key)
		}
		if thread := headers.Get("thread_id"); thread != "" {
			headers.Set("x-client-request-id", thread)
		}
	}
	if ua != "" {
		originator, paired, ok := openai.PairCodexClientIdentity(ua)
		if ok {
			headers.Set("User-Agent", paired)
			headers.Set("originator", originator)
			headers.Set("version", version)
		}
	}
}

func codexDiagnosticDial(ctx context.Context, dialer openAIWSClientDialer, variant, address string, headers http.Header) (openAIWSClientConn, int, http.Header, error) {
	if variant != "ws_no_compression" {
		return dialer.Dial(ctx, address, headers, "")
	}
	base, ok := dialer.(*coderOpenAIWSClientDialer)
	if !ok {
		return nil, 0, nil, errors.New("diagnostic requires the configured coder WS dialer")
	}
	conn, response, err := coderws.Dial(ctx, address, &coderws.DialOptions{
		HTTPClient: base.directClient, HTTPHeader: headers.Clone(), CompressionMode: coderws.CompressionDisabled,
	})
	status := 0
	responseHeaders := make(http.Header)
	if response != nil {
		status, responseHeaders = response.StatusCode, response.Header.Clone()
	}
	if err != nil {
		var body []byte
		if response != nil && response.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(response.Body, 8192))
			_ = response.Body.Close()
		}
		return nil, status, responseHeaders, &openAIWSHandshakeError{Body: body, Err: err}
	}
	conn.SetReadLimit(base.upstreamReadLimitBytes)
	return &coderOpenAIWSClientConn{conn: conn}, status, responseHeaders, nil
}

func codexDiagnosticModels(t *testing.T, client *http.Client, svc *OpenAIGatewayService, account *Account, version string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req, err := svc.buildUpstreamRequest(ctx, c, account, nil, account.GetCredential("access_token"), false, "", true)
	if err != nil {
		t.Fatal("could not prepare models diagnostic")
	}
	req.Method, req.Body, req.ContentLength = http.MethodGet, nil, 0
	req.URL.Path, req.URL.RawQuery = "/backend-api/codex/models", "client_version="+version
	req.Header.Set("Accept", "application/json")
	result := map[string]any{"kind": "model_manifest", "account_id": account.ID}
	response, err := client.Do(req)
	if err == nil {
		result["http_status"] = response.StatusCode
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK {
			models := []map[string]any{}
			for _, model := range gjson.GetBytes(body, "models").Array() {
				row := map[string]any{}
				for _, field := range []string{"slug", "supported_in_api", "visibility", "use_responses_lite", "supports_parallel_tool_calls", "prefer_websockets", "safety_buffering"} {
					if value := model.Get(field); value.Exists() {
						row[field] = value.Value()
					}
				}
				models = append(models, row)
			}
			result["models"] = models
		} else {
			codexDiagnosticHTTPError(response.Header, body, result)
		}
	} else {
		result["error"] = strings.ReplaceAll(err.Error(), account.GetCredential("access_token"), "[REDACTED]")
	}
	encoded, _ := json.Marshal(result)
	fmt.Println(strings.ReplaceAll(string(encoded), account.GetCredential("access_token"), "[REDACTED]"))
}

func codexDiagnosticEvent(raw []byte, started time.Time, result map[string]any) bool {
	typeName := gjson.GetBytes(raw, "type").String()
	if typeName == "response.metadata" && gjson.GetBytes(raw, "metadata.type").String() == "safety_buffering" {
		result["safety_buffering_use_cases"] = gjson.GetBytes(raw, "metadata.use_cases").Value()
		result["safety_buffering_reasons"] = gjson.GetBytes(raw, "metadata.reasons").Value()
		result["safety_buffering_retry_model"] = firstValidTrimmedGJSONString(raw, "metadata.retry_model", "metadata.faster_model")
	}
	if typeName == "response.created" && gjson.GetBytes(raw, "response.safety_buffering").Exists() {
		result["safety_buffering_use_cases"] = gjson.GetBytes(raw, "response.safety_buffering.use_cases").Value()
		result["safety_buffering_reasons"] = gjson.GetBytes(raw, "response.safety_buffering.reasons").Value()
		result["safety_buffering_retry_model"] = firstValidTrimmedGJSONString(raw, "response.safety_buffering.retry_model", "response.safety_buffering.faster_model")
	}
	if typeName == "response.output_text.delta" {
		if _, exists := result["first_text_ms"]; !exists {
			result["first_text_ms"] = time.Since(started).Milliseconds()
		}
	}
	switch typeName {
	case "response.completed":
		result["success"] = true
		result["response_model"] = gjson.GetBytes(raw, "response.model").String()
		result["input_tokens"] = gjson.GetBytes(raw, "response.usage.input_tokens").Int()
		result["cached_tokens"] = gjson.GetBytes(raw, "response.usage.input_tokens_details.cached_tokens").Int()
		result["output_tokens"] = gjson.GetBytes(raw, "response.usage.output_tokens").Int()
		return true
	case "response.failed", "response.incomplete", "error":
		result["event"] = typeName
		for _, field := range []string{"response.error", "error", "response.incomplete_details"} {
			if value := gjson.GetBytes(raw, field); value.Exists() {
				result["error"] = value.Value()
				break
			}
		}
		return true
	}
	return false
}

func codexDiagnosticHTTPError(headers http.Header, body []byte, result map[string]any) {
	result["error_body_bytes"] = len(body)
	result["error_content_type"] = headers.Get("Content-Type")
	result["error_server"] = headers.Get("Server")
	result["edge_mitigation"] = headers.Get("Cf-Mitigated")
	result["retry_after"] = headers.Get("Retry-After")
	if strings.HasPrefix(headers.Get("Content-Type"), "text/plain") {
		text := strings.TrimSpace(string(body))
		if len(text) > 300 {
			text = text[:300]
		}
		result["plain_error"] = text
	}
	for _, field := range []string{"error.message", "detail", "message"} {
		if value := gjson.GetBytes(body, field).String(); value != "" {
			if len(value) > 300 {
				value = value[:300]
			}
			result["error"] = value
			return
		}
	}
	lower := strings.ToLower(string(body))
	if start := strings.Index(lower, "<title>"); start >= 0 {
		if end := strings.Index(lower[start+7:], "</title>"); end >= 0 && end <= 150 {
			result["error_page_title"] = string(body[start+7 : start+7+end])
		}
	}
	result["non_json_error"] = !gjson.ValidBytes(body)
	result["challenge_page"] = strings.Contains(lower, "cf-chl-") || strings.Contains(lower, "just a moment")
}

//go:build codexdiagnostic

package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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
		Rounds           int
		DefaultFull      bool
		InspectModels    bool
		FreshConnections bool
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		t.Fatal("invalid diagnostic input")
	}
	if len(input.Accounts) == 0 || len(input.Accounts) > 3 || len(input.Models) == 0 || len(input.Models) > 2 || input.Rounds < 1 || input.Rounds > 3 || NormalizeCodexClientVersion(input.Version) == "" {
		t.Fatal("invalid diagnostic bounds")
	}
	if len(input.Transports) == 0 || len(input.Transports) > 2 {
		t.Fatal("choose HTTP and/or WS")
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
	sessions := make(map[int64]string)
	retryAfter := make(map[int64]time.Time)
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
		clients[account.ID] = &http.Client{Transport: transport}
		http1Transport := codexDiagnosticHTTP1Transport(transport)
		t.Cleanup(http1Transport.CloseIdleConnections)
		http1Clients[account.ID] = &http.Client{Transport: http1Transport}
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
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						session := sessions[account.ID]
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
						observation["fixed_egress"] = input.SourceIPv6 != ""
						observation["fresh_connection_policy"] = input.FreshConnections
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
				for _, field := range []string{"slug", "supported_in_api", "visibility", "use_responses_lite", "supports_parallel_tool_calls", "prefer_websockets"} {
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

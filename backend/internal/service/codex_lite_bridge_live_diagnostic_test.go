//go:build codexdiagnostic

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// This diagnostic uses the real bridge and an isolated pool, without scheduling,
// persistent account changes, HTTP fallback, or automatic retries.
func TestCodexLiteBridgeLiveDiagnostic(t *testing.T) {
	if os.Getenv("CODEX_HEADER_DIAGNOSTIC") != "1" {
		t.Skip("live diagnostic requires explicit enablement and snapshots")
	}
	var input struct {
		Accounts   []*Account
		Models     []string
		Version    string
		SourceIPv6 string
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		t.Fatal("invalid diagnostic input")
	}
	if len(input.Accounts) == 0 || len(input.Accounts) > 2 || len(input.Models) != 1 || NormalizeCodexClientVersion(input.Version) == "" {
		t.Fatal("invalid diagnostic bounds")
	}
	address := net.ParseIP(input.SourceIPv6)
	if address == nil || address.To4() != nil {
		t.Fatal("diagnostic requires an assigned IPv6 source")
	}
	SetCodexCanonicalUserAgentResolver(func() string { return buildCodexCLIUserAgent(input.Version) })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	for _, account := range input.Accounts {
		if account == nil || !account.IsOpenAIOAuthLike() || account.GetCredential("access_token") == "" || account.ProxyID != nil {
			t.Fatal("diagnostic requires direct authorized OAuth/PAT snapshots")
		}
		started := time.Now()
		result := map[string]any{"account_id": account.ID, "model": input.Models[0], "kind": "lite_bridge_tools", "success": false}
		cfg := newOpenAIWSV2TestConfig()
		cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
		cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
		cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
		netDialer := &net.Dialer{Timeout: 10 * time.Second, LocalAddr: &net.TCPAddr{IP: address}}
		transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, DialContext: func(ctx context.Context, _ string, addr string) (net.Conn, error) {
			return netDialer.DialContext(ctx, "tcp6", addr)
		}}
		pool := newOpenAIWSConnPool(cfg)
		pool.setClientDialerForTest(&coderOpenAIWSClientDialer{
			directClient: &http.Client{Transport: transport}, proxyClients: make(map[string]*openAIWSProxyClientEntry),
			upstreamReadLimitBytes: openAIWSUpstreamReadLimitBytesDefault,
		})
		svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, toolCorrector: NewCodexToolCorrector()}
		session := uuid.NewString()
		body := createOpenAITestPayload(input.Models[0], true)
		body["prompt_cache_key"] = session
		body["client_metadata"] = map[string]any{"session_id": session}
		body["input"] = []map[string]any{{"role": "user", "content": "Call deployment_echo once with value OK. After its result, reply OK."}}
		body["tools"] = []any{map[string]any{"type": "function", "name": "deployment_echo", "description": "Return the supplied value.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}, "additionalProperties": false}}}
		body["tool_choice"] = map[string]any{"type": "function", "name": "deployment_echo"}
		body["include"] = []string{"reasoning.encrypted_content"}
		codexDiagnosticLitePayload(t, body, "http")
		for turn := range 2 {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", buildCodexCLIUserAgent(input.Version))
			c.Request.Header.Set("session-id", session)
			c.Request.Header.Set("thread-id", session)
			c.Request.Header.Set(responsesLiteHeader, "true")
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			ids := resolveCodexFingerprintIDsFromRequestWithDefault(account, c.Request.Header, true)
			stageCodexFingerprintIDs(c, ids)
			request := maps.Clone(body)
			applyCodexAccountIdentityClientMetadataMap(request, account, 0)
			applyCodexFingerprintClientMetadata(request, ids)
			recovered := false
			forwarded, err := svc.forwardOpenAIWSV2(ctx, c, account, request, session, account.GetCredential("access_token"),
				OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, true,
				input.Models[0], input.Models[0], time.Now(), 1, "", &recovered)
			cancel()
			result["turns_attempted"] = turn + 1
			if err != nil {
				result["error"] = strings.ReplaceAll(err.Error(), account.GetCredential("access_token"), "[REDACTED]")
				break
			}
			var completed gjson.Result
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if raw, ok := strings.CutPrefix(line, "data:"); ok && gjson.Get(raw, "type").String() == "response.completed" {
					completed = gjson.Get(raw, "response")
				}
			}
			if forwarded == nil || !forwarded.OpenAIWSMode || !completed.Exists() {
				result["error"] = "bridge did not complete over WebSocket"
				break
			}
			result["websocket"] = true
			if turn == 0 {
				call := completed.Get(`output.#(type=="function_call")`)
				if call.Get("name").String() != "deployment_echo" || call.Get("call_id").String() == "" || gjson.Get(call.Get("arguments").String(), "value").String() != "OK" {
					result["error"] = "missing expected function invocation"
					break
				}
				result["tool_called"] = true
				var history []any
				encoded, _ := json.Marshal(body["input"])
				if err := json.Unmarshal(encoded, &history); err != nil {
					t.Fatal("invalid diagnostic history")
				}
				for _, item := range completed.Get("output").Array() {
					history = append(history, item.Value())
				}
				body["input"] = append(history, map[string]any{"type": "function_call_output", "call_id": call.Get("call_id").String(), "output": "OK"})
				body["tool_choice"] = "none"
			} else {
				answer := completed.Get(`output.#(type=="message").content.#(type=="output_text").text`).String()
				result["success"] = strings.Contains(answer, "OK")
				result["continuation"] = result["success"]
			}
		}
		pool.Close()
		transport.CloseIdleConnections()
		result["duration_ms"] = time.Since(started).Milliseconds()
		encoded, _ := json.Marshal(result)
		fmt.Println(string(encoded))
	}
}

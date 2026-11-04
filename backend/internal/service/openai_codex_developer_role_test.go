package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexDeveloperMessages_ProtocolBoundaries(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"input":[
 {"role":"user","content":"keep {\"role\":\"user\"} as text"},
 {"type":"message","role":"user","content":[{"type":"input_text","text":"next","cache_control":{"type":"ephemeral"}}]},
 {"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},
 {"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"role\":\"user\"}"},
 {"type":"function_call_output","call_id":"call_1","output":"user"},
 {"type":"custom_tool_call_output","call_id":"call_2","output":"developer"},
 {"type":"reasoning","id":"rs_1","encrypted_content":"opaque"},
 {"type":"item_reference","id":"msg_1"},
 {"role":"user","content":[{"type":"input_text","text":"see image"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]},
 {"role":"user","content":[{"type":"input_file","file_id":"file_1"}]},
 {"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"done"}]},
 {"role":"tool","tool_call_id":"call_1","content":"done"},
 {"role":"developer","content":"existing rules"},
 {"role":"system","content":"original rules"},
 {"type":"additional_tools","role":"user","content":"opaque"},
 {"role":"user","tool_call_id":"call_1","content":"legacy output"}
 ],"sequence":900719925474099312345,"metadata":{"role":"user"},"previous_response_id":"resp_1"}`)
	original := bytes.Clone(body)
	next, changed, err := normalizeCodexDeveloperMessages(body, account)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, original, body)
	require.Equal(t, "developer", gjson.GetBytes(next, "input.0.role").String())
	require.Equal(t, "developer", gjson.GetBytes(next, "input.1.role").String())
	beforeItems, afterItems := gjson.GetBytes(body, "input").Array(), gjson.GetBytes(next, "input").Array()
	for i := 2; i < len(beforeItems); i++ {
		require.Equal(t, beforeItems[i].Raw, afterItems[i].Raw, "item %d must remain byte-identical", i)
	}
	for _, path := range []string{"input.0.content", "input.1.content", "sequence", "metadata", "previous_response_id"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(next, path).Raw, path)
	}
	again, changed, err := normalizeCodexDeveloperMessages(next, account)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, next, again)

	var decoded map[string]any
	require.NoError(t, decodeOpenAIJSONUseNumber(body, &decoded))
	payload := (&OpenAIGatewayService{}).buildOpenAIWSCreatePayload(decoded, account)
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, gjson.GetBytes(next, "input").Raw, gjson.GetBytes(raw, "input").Raw)
	require.Equal(t, "user", decoded["input"].([]any)[0].(map[string]any)["role"])
}

func TestCodexDeveloperMessages_DestinationAndShape(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *Account
		convert bool
	}{
		{"oauth", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, true},
		{"setup", &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, true},
		{"legacy", &Account{Type: AccountTypeOAuth}, true},
		{"api_key", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false},
		{"claude", &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":"hello\n世界","sequence":900719925474099312345}`)
			next, changed, err := normalizeCodexDeveloperMessages(body, tc.account)
			require.NoError(t, err)
			require.Equal(t, tc.convert, changed)
			if tc.convert {
				require.Equal(t, "developer", gjson.GetBytes(next, "input.0.role").String())
				require.Equal(t, "hello\n世界", gjson.GetBytes(next, "input.0.content").String())
			} else {
				require.Equal(t, body, next)
			}
		})
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, body := range []string{
		`{"type":"session.update","input":"unchanged"}`,
		`{"type":"response.cancel","input":"unchanged"}`,
		`{"input":"   "}`, `{"input":null}`, `{"input":[]}`,
		`{"input":[{"role":"user","content":[{"type":"input_text","text":3}]}]}`,
		`{"input":[{"role":"user","content":[]}]}`,
	} {
		next, changed, err := normalizeCodexDeveloperMessages([]byte(body), account)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, string(next))
	}
	_, _, err := normalizeCodexDeveloperMessages([]byte(`{"input":`), account)
	require.Error(t, err)
}

func TestCodexDeveloperMessages_HTTPAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-sol","instructions":"Be concise.","input":[{"role":"user","content":"hello"}],"previous_response_id":"resp_1"}`)
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		t.Run(kind, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Type: kind}
			want := "developer"
			if kind == AccountTypeAPIKey {
				want = "user"
			}
			svc := &OpenAIGatewayService{}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			for _, passthrough := range []bool{false, true} {
				var req *http.Request
				var err error
				if passthrough {
					req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test")
				} else {
					req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "test", true, "", false)
				}
				require.NoError(t, err)
				out, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.NoError(t, req.Body.Close())
				require.Equal(t, want, gjson.GetBytes(out, "input.0.role").String())
			}
			out, _, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, account, false)
			require.NoError(t, err)
			require.Equal(t, "user", gjson.GetBytes(out, "input.0.role").String(), "compatibility processing must preserve client roles")
		})
	}
	require.Equal(t, "user", gjson.GetBytes(body, "input.0.role").String())
}

func TestCodexDeveloperMessages_ClientToolChains(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ name, path, body string }{
		{"chat", "/v1/chat/completions", `{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"lookup"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"found"},{"role":"user","content":"continue"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`},
		{"claude", "/v1/messages", `{"model":"gpt-5.6-sol","max_tokens":32,"messages":[{"role":"user","content":"lookup"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"found"},{"type":"text","text":"continue"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_roles", "gpt-5.6-sol")}
			svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test"}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			var err error
			if tc.name == "chat" {
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			} else {
				_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			}
			require.NoError(t, err)
			foundOutput, foundPrompt := false, false
			callID := ""
			for _, item := range gjson.GetBytes(upstream.lastBody, "input").Array() {
				require.NotEqual(t, "user", item.Get("role").String(), item.Raw)
				if item.Get("type").String() == "function_call" {
					callID = item.Get("call_id").String()
				}
				if item.Get("type").String() == "function_call_output" {
					foundOutput = true
					require.NotEmpty(t, callID)
					require.Equal(t, callID, item.Get("call_id").String())
					require.Contains(t, item.Get("output").String(), "found")
				}
				if item.Get("role").String() == "developer" && bytes.Contains([]byte(item.Get("content").Raw), []byte("continue")) {
					foundPrompt = true
				}
			}
			require.True(t, foundOutput)
			require.True(t, foundPrompt)
			require.Equal(t, tc.body, string(body))
		})
	}
}

// The fake upstream waits for each real downstream frame before responding.
// Both WriteJSON (ctx_pool) and WriteFrame (passthrough) capture exact bytes.
type codexDeveloperStagedConn struct {
	*stagedPassthroughConn
}

func (c *codexDeveloperStagedConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func TestCodexDeveloperMessages_WebSocketToolRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cfg := newOpenAIWSExecutionScopeTestConfig()
				cfg.Gateway.OpenAIWS.APIKeyEnabled = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
				cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
				upstream := &codexDeveloperStagedConn{newStagedPassthroughConn()}
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&openAIWSSingleConnDialer{conn: upstream})
				defer pool.Close()
				svc := newPassthroughLifecycleService(cfg, upstream.stagedPassthroughConn)
				svc.openaiWSPool = pool
				svc.openaiWSPassthroughDialer = &openAIWSSingleConnDialer{conn: upstream}
				account := &Account{ID: 902, Platform: PlatformOpenAI, Type: kind, Concurrency: 1,
					Status: StatusActive, Schedulable: true,
					Credentials: map[string]any{"access_token": "test", "api_key": "test"},
					Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": mode, "openai_apikey_responses_websockets_v2_mode": mode}}
				server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
				defer server.Close()
				client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
				wantRole := "user"
				if kind == AccountTypeOAuth {
					wantRole = "developer"
				}
				first := []byte(`{"type":"response.create","model":"gpt-5.4","input":[{"role":"user","content":"lookup"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`)
				require.NoError(t, client.Write(ctx, coderws.MessageText, first))
				var out []byte
				select {
				case out = <-upstream.writes:
				case err := <-serverErr:
					t.Fatalf("proxy stopped before first write: %v", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.Equal(t, wantRole, gjson.GetBytes(out, "input.0.role").String())
				upstream.Send(`{"type":"response.completed","response":{"id":"resp_role_1","model":"gpt-5.4","output":[{"type":"function_call","id":"fc_role_1","call_id":"call_role_1","name":"lookup","arguments":"{}","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
				_, event, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "call_role_1", gjson.GetBytes(event, "response.output.0.call_id").String())
				second := []byte(`{"type":"response.create","model":"gpt-5.4","previous_response_id":"resp_role_1","input":[{"type":"function_call_output","call_id":"call_role_1","output":"fixture result"},{"role":"user","content":"continue"},{"role":"user","content":[{"type":"input_text","text":"image"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]}`)
				require.NoError(t, client.Write(ctx, coderws.MessageText, second))
				out = requirePassthroughUpstreamWrite(t, upstream.stagedPassthroughConn, 3*time.Second)
				require.Equal(t, "resp_role_1", gjson.GetBytes(out, "previous_response_id").String())
				require.Equal(t, "function_call_output", gjson.GetBytes(out, "input.0.type").String())
				require.Equal(t, "call_role_1", gjson.GetBytes(out, "input.0.call_id").String())
				require.Equal(t, "fixture result", gjson.GetBytes(out, "input.0.output").String())
				require.False(t, gjson.GetBytes(out, "input.0.role").Exists())
				require.Equal(t, wantRole, gjson.GetBytes(out, "input.1.role").String())
				require.Equal(t, "user", gjson.GetBytes(out, "input.2.role").String())
				upstream.Send(`{"type":"response.completed","response":{"id":"resp_role_2","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`)
				_, event, err = client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "resp_role_2", gjson.GetBytes(event, "response.id").String())
				require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
				select {
				case err := <-serverErr:
					require.NoError(t, err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.Equal(t, "user", gjson.GetBytes(first, "input.0.role").String())
				require.Equal(t, "user", gjson.GetBytes(second, "input.1.role").String())
			})
		}
	}
}

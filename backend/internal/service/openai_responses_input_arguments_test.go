package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesInputArgumentsPreserveWireValues(t *testing.T) {
	const object = `{ "id":9007199254740993,"n":1.234567890123456789,"exponent":1e100,"text":"你好" }`
	prefix := `{"model":"gpt-6-astra","prompt_cache_key":"session","metadata":{"id":9007199254740993},"tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec","format":{"type":"text"}}]}],"input":[`
	item := `{"type":"function_call","call_id":"call_1","name":"lookup","namespace":"functions","arguments":`
	body := []byte(prefix + item + object + `},{"type":"custom_tool_call","name":"exec","input":"text(\"OK\");","arguments":{"untouched":true}},{"type":"function_call","call_id":"call_2","name":"next","arguments":{}}]}`)
	normalized, changed, err := normalizeOpenAIResponsesInputArguments(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, object, gjson.GetBytes(normalized, "input.0.arguments").String())
	require.Equal(t, gjson.String, gjson.GetBytes(normalized, "input.2.arguments").Type)
	require.Equal(t, "{}", gjson.GetBytes(normalized, "input.2.arguments").String())
	for _, path := range []string{"model", "prompt_cache_key", "metadata", "tools", "input.1", "input.0.call_id", "input.0.namespace"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(normalized, path).Raw, path)
	}
	second, changed, err := normalizeOpenAIResponsesInputArguments(normalized)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, normalized, second)
	for _, value := range []string{`"{\"query\":\"keep  spaces\"}"`, `null`, `[]`, `false`, `42`} {
		original := []byte(prefix + item + value + `}]}`)
		result, changed, err := normalizeOpenAIResponsesInputArguments(original)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, original, result)
	}
}

func TestForwardStringifiesInput23ArgumentsBeforeUpstream(t *testing.T) {
	const arguments = `{"id":9007199254740993,"query":"hello"}`
	body := []byte(`{"model":"gpt-6-astra","stream":true,"store":false,"instructions":"test","input":[` +
		strings.Repeat(`{"role":"user","content":"history"},`, 23) +
		`{"type":"function_call","name":"lookup","call_id":"fc_23","arguments":` + arguments + `},` +
		`{"type":"function_call_output","call_id":"fc_23","output":"ok"},{"role":"user","content":"continue"}]}`)
	for _, kind := range []string{"oauth", "apikey", "oauth_passthrough", "siwc"} {
		t.Run(kind, func(t *testing.T) {
			account := newOpenAIOAuthNamespaceTestAccount()
			switch kind {
			case "apikey":
				account = newOpenAIRejectedFieldTestAccount()
			case "oauth_passthrough":
				account.Extra = map[string]any{"openai_passthrough": true}
			case "siwc":
				account = siwcFixtureAccount()
			}
			response := newOpenAIRejectedFieldTestResponse(http.StatusOK, siwcForwardSSE)
			response.Header.Set("Content-Type", "text/event-stream")
			upstream := &httpUpstreamRecorder{responses: []*http.Response{response}}
			service := newOpenAIRejectedFieldTestService(upstream)
			var siwcBodies [][]byte
			service.siwcTransport = siwcCatalogRoundTripper(func(request *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				siwcBodies = append(siwcBodies, data)
				return response, nil
			})
			result, err := service.Forward(context.Background(), newOpenAIRejectedFieldTestContext(body), account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			bodies := upstream.bodies
			if kind == "siwc" {
				bodies = siwcBodies
			}
			require.Len(t, bodies, 1, "repair before the first attempt, without an upstream 400 retry")
			arg := gjson.GetBytes(bodies[0], "input.23.arguments")
			require.Equal(t, gjson.String, arg.Type)
			require.Equal(t, arguments, arg.String())
			require.Equal(t, "fc_23", gjson.GetBytes(bodies[0], "input.23.call_id").String())
			require.Equal(t, "fc_23", gjson.GetBytes(bodies[0], "input.24.call_id").String())
		})
	}
}

func TestResponsesCompatibilityStringifiesFunctionArguments(t *testing.T) {
	const arguments = `{"id":9007199254740993,"nested":{"text":"你好\n\"quoted\"","arguments":{"keep":true}},"items":[1,null,false]}`
	body := []byte(`{"model":"gpt-6-astra","store":false,"input":[{"type":"function_call","call_id":"call_1","name":"lookup","namespace":"functions","arguments":` + arguments + `},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	for _, account := range []*Account{
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	} {
		t.Run(account.Type, func(t *testing.T) {
			for _, lite := range []bool{false, true} {
				normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, account, lite)
				require.NoError(t, err)
				require.True(t, changed)
				arg := gjson.GetBytes(normalized, "input.0.arguments")
				require.Equal(t, gjson.String, arg.Type)
				require.Equal(t, arguments, arg.String())
				require.Equal(t, "functions", gjson.GetBytes(normalized, "input.0.namespace").String())
				require.Equal(t, "call_1", gjson.GetBytes(normalized, "input.0.call_id").String())
			}
		})
	}
}

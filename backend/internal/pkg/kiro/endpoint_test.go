package kiro

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKiroEndpointSelectionAndFingerprint(t *testing.T) {
	for _, endpoint := range []string{"", "ide", "runtime_v3", "cli"} {
		c, err := Normalize(map[string]any{"kiroApiKey": "ksk_test_only", "endpoint": endpoint, "machineId": "test-machine-identity"})
		require.NoError(t, err)
		h := http.Header{}
		OperationHeaders(h, c, "ksk_test_only", OperationGeneration)
		require.Equal(t, "Bearer ksk_test_only", h.Get("Authorization"))
		require.Equal(t, "API_KEY", h.Get("tokentype"))
		require.Equal(t, "true", h.Get("x-amzn-codewhisperer-optout"))
		require.Equal(t, "attempt=1; max=1", h.Get("amz-sdk-request"))
		if endpoint == "cli" {
			require.Equal(t, "https://q.us-east-1.amazonaws.com/", Endpoint(c))
			require.Equal(t, "AmazonCodeWhispererStreamingService.GenerateAssistantResponse", h.Get("X-Amz-Target"))
			require.Contains(t, h.Get("User-Agent"), "aws-sdk-rust/"+CLISDKVersion)
			require.Equal(t, "cli", AccountEnvironment(c).Endpoint)
			require.Equal(t, CLISDKVersion, AccountEnvironment(c).StreamingSDK)
		} else {
			require.Equal(t, "https://runtime.us-east-1.kiro.dev/", Endpoint(c))
			require.Equal(t, "KiroRuntimeService.GenerateAssistantResponse", h.Get("X-Amz-Target"))
			require.Equal(t, "runtime_v3", AccountEnvironment(c).Endpoint)
		}
		generationKey := EnvironmentKey(c)
		c["endpoint"] = "cli"
		if endpoint != "cli" {
			require.NotEqual(t, generationKey, EnvironmentKey(c))
		}
		OperationHeaders(h, c, "ksk_test_only", OperationModels)
		require.Equal(t, "AmazonCodeWhispererService.ListAvailableModels", h.Get("X-Amz-Target"))
		require.NotContains(t, h.Get("User-Agent"), "aws-sdk-rust")
	}
	for _, endpoint := range []any{"https://untrusted.invalid", "other", true, 5} {
		_, err := Normalize(map[string]any{"kiroApiKey": "ksk_test_only", "endpoint": endpoint})
		require.Error(t, err)
	}
}

func TestCLIEnvelopePreservesUserAndToolContent(t *testing.T) {
	raw := []byte(`{"model":"claude-opus-5-5","max_tokens":100,"system":"Keep AI_EDITOR text unchanged","messages":[{"role":"user","content":"AI_EDITOR"},{"role":"assistant","content":[{"type":"tool_use","id":"call_one","name":"read_probe","input":{"origin":"AI_EDITOR"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_one","content":"AI_EDITOR result"}]}],"tools":[{"name":"read_probe","description":"AI_EDITOR description","input_schema":{"type":"object","properties":{"origin":{"type":"string","enum":["AI_EDITOR"]}}}}]}`)
	for _, method := range []string{"api_key", "social"} {
		c := map[string]any{"endpoint": "cli", "auth_method": method, "profile_arn": "test-profile"}
		wire, _, err := Convert(raw, "claude-opus-5-5", "stable-session", c)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(wire, &body))
		if method == "api_key" {
			require.NotContains(t, body, "profileArn")
			require.NotContains(t, UsageURL(c), "profileArn")
		} else {
			require.Equal(t, "test-profile", body["profileArn"])
		}
		state := body["conversationState"].(map[string]any)
		require.Equal(t, "stable-session", state["conversationId"])
		require.NotContains(t, state, "agentContinuationId")
		current := state["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
		require.Equal(t, "KIRO_CLI", current["origin"])
		require.Equal(t, "claude-opus-5.5", current["modelId"])
		history := state["history"].([]any)
		old := history[0].(map[string]any)["userInputMessage"].(map[string]any)
		require.Equal(t, "KIRO_CLI", old["origin"])
		require.NotContains(t, old, "modelId")
		for _, content := range []string{"Keep AI_EDITOR text unchanged", "AI_EDITOR result", "AI_EDITOR description", `"origin":"AI_EDITOR"`, `"enum":["AI_EDITOR"]`} {
			require.Contains(t, string(wire), content)
		}
	}
}

package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiRequestPreservesToolHistoryAndSchema(t *testing.T) {
	raw := []byte(`{"systemInstruction":{"parts":[{"text":"Be brief"}]},"contents":[{"role":"user","parts":[{"text":"weather"}]},{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"Paris"}}}]},{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"temperature":20}}}]}],"tools":[{"functionDeclarations":[{"name":"weather","parameters":{"type":"OBJECT","properties":{"city":{"type":"STRING"}}}}]}]}`)
	converted, err := GeminiRequest(raw, "claude-haiku-4.5")
	require.NoError(t, err)
	var req Request
	require.NoError(t, json.Unmarshal(converted, &req))
	require.Len(t, req.Messages, 3)
	calls, err := Blocks(req.Messages[1].Content)
	require.NoError(t, err)
	results, err := Blocks(req.Messages[2].Content)
	require.NoError(t, err)
	require.Equal(t, calls[0].ID, results[0].ToolUseID)
	require.Contains(t, string(req.Tools[0].InputSchema), `"type":"object"`)
	require.Contains(t, string(req.Tools[0].InputSchema), `"type":"string"`)
}
func TestGeminiRequestRejectsUnsupportedControls(t *testing.T) {
	for _, field := range []string{`"cachedContent":"x"`, `"generationConfig":{"candidateCount":2}`, `"generationConfig":{"responseMimeType":"application/json"}`, `"tools":[{"googleSearch":{}}]`, `"toolConfig":{"functionCallingConfig":{"mode":"ANY"}}`} {
		_, err := GeminiRequest([]byte(`{"contents":[{"parts":[{"text":"hi"}]}],`+field+`}`), "claude-haiku-4.5")
		require.Error(t, err)
	}
}

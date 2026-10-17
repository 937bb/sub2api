package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCHistoryKeepsToolCorrelationsAndOpaqueReasoning(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","input":[
{"type":"additional_tools","tools":[{"type":"custom","name":"exec"}]},
{"type":"message","id":"item_old_message","role":"assistant","content":[{"type":"output_text","text":"Working"}]},
{"type":"reasoning","id":"rs_old","encrypted_content":"opaque","summary":[]},
{"type":"custom_tool_call","id":"ctc_old","call_id":"call_custom","name":"exec","input":"marker"},
{"type":"custom_tool_call_output","call_id":"call_custom","output":"OK"},
{"type":"function_call","id":"fc_old","call_id":"call_function","name":"probe_json","arguments":"{}"},
{"type":"function_call_output","call_id":"call_function","output":"OK"},
{"type":"message","id":"msg_valid","role":"assistant","content":[{"type":"output_text","text":"Done"}]}]}`)
	normalized, err := normalizeSIWCResponsesBody(body)
	require.NoError(t, err)
	require.Equal(t, "developer", gjson.GetBytes(normalized, "input.0.role").String())
	require.False(t, gjson.GetBytes(normalized, "input.1.id").Exists())
	require.Equal(t, "Working", gjson.GetBytes(normalized, "input.1.content.0.text").String())
	for _, index := range []string{"2", "3", "4", "5", "6", "7"} {
		require.JSONEq(t, gjson.GetBytes(body, "input."+index).Raw, gjson.GetBytes(normalized, "input."+index).Raw)
	}
	again, err := normalizeSIWCResponsesBody(normalized)
	require.NoError(t, err)
	require.JSONEq(t, string(normalized), string(again))
}

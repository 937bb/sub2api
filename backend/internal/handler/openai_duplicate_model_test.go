package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"testing"
)

func TestOpenAIResponsesWebSocketAmbiguousModelWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		for _, payload := range []string{
			`{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1"}`,
			`{"type":"response.create","model":"gpt-5.4","Model":"gpt-4.1"}`,
			`{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4"}`,
			`{"type":"response.create","model":"gpt-5.4","session":{"model":"x","model":"y"}}`,
			`{"type":"session.update","type":"response.create","model":"gpt-5.4"}`,
		} {
			t.Run(mode+"/"+payload, func(t *testing.T) {
				runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
					firstPayload: payload, group: wsAllowlistGroup(false), ingressMode: mode,
					firstFrameCloseExpected: true, closeReason: "ambiguous model",
				})
				runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
					firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
					secondPayload: payload, group: wsAllowlistGroup(false), ingressMode: mode,
					secondTurnCloseExpected: true, closeReason: "ambiguous model",
				})
			})
		}
	}
}

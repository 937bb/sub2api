package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func wsAllowlistGroup(enabled bool, models ...string) *service.Group {
	return &service.Group{
		ID:       4201,
		Platform: service.PlatformOpenAI,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: enabled,
			Models:  models,
		},
	}
}

func TestOpenAIResponsesWebSocket_FirstFrameModelNotAllowedCloses_Passthrough(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_FirstFrameModelNotAllowedCloses_NativeIngress(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModeDedicated,
		firstFrameCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_FirstFrameAllowlistedModelProceeds(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		group:        wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 1 {
		t.Fatalf("expected one completed event, got %d", len(got.clientEvents))
	}
}

func TestOpenAIResponsesWebSocket_SubsequentTurnModelNotAllowedCloses_Passthrough(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload:           `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		secondTurnCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_SubsequentTurnModelNotAllowedCloses_NativeIngress(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload:           `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModeDedicated,
		secondTurnCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_SubsequentTurnOmittedModelUsesSessionModel(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload: `{"type":"response.create","stream":false}`,
		group:         wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
	if len(got.upstreamPayloads) != 2 {
		t.Fatalf("expected two upstream frames, got %d", len(got.upstreamPayloads))
	}
}

func TestOpenAIResponsesWebSocket_DisabledAllowlistDoesNotInterfere(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		secondPayload: `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		group:         wsAllowlistGroup(false, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
}

func TestOpenAIResponsesWebSocket_FirstFrameDuplicateModelKeysRejected(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:                   wsAllowlistGroup(true, "gpt-5.4"),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
				closeReason:             "ambiguous model",
			})
		})
	}
}

func TestOpenAIResponsesWebSocket_FirstFrameCaseVariantModelKeyRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","Model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
		closeReason:             "ambiguous model",
	})
}

func TestOpenAIResponsesWebSocket_SubsequentTurnDuplicateModelKeysRejected(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:                   wsAllowlistGroup(true, "gpt-5.4"),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
				closeReason:             "ambiguous model",
			})
		})
	}
}

func TestOpenAIResponsesWebSocket_DuplicateIdenticalModelKeysRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4","stream":false}`,
		group:        wsAllowlistGroup(true, "gpt-5.4"), firstFrameCloseExpected: true, closeReason: "ambiguous model",
	})
}

func TestOpenAIResponsesWebSocket_SessionUpdateRotationBypassRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		midPayload:              `{"type":"session.update","session":{"model":"gpt-4.1"}}`,
		secondPayload:           `{"type":"response.create","session":{"model":"gpt-5.4"},"stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		secondTurnCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_SessionUpdateToAllowedModelStillWorks(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		midPayload:    `{"type":"session.update","session":{"model":"gpt-5.4"}}`,
		secondPayload: `{"type":"response.create","stream":false}`,
		group:         wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 || len(got.logs) != 2 || len(got.upstreamPayloads) != 3 {
		t.Fatalf("expected two completed/billed turns and three upstream frames, got events=%d logs=%d frames=%d", len(got.clientEvents), len(got.logs), len(got.upstreamPayloads))
	}
}

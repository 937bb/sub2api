package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// shouldPrewarmOpenAIWSSession enables the protocol-level warmup for quota
// bypass accounts even when the general prewarm switch remains disabled.
// Explicitly enabling the general switch keeps the same behavior available for
// ordinary OpenAI WebSocket sessions.
func (s *OpenAIGatewayService) shouldPrewarmOpenAIWSSession(hooks *OpenAIWSIngressHooks) bool {
	if hooks != nil && hooks.QuotaBypassEnabled {
		return true
	}
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.PrewarmGenerateEnabled
}

// quotaBypassPrewarmPayload adds the same synthetic tool turn used by the
// actual request without invoking the usage callback. The warmup is upstream
// session setup and must not be billed as a client turn.
func quotaBypassPrewarmPayload(payload []byte, hooks *OpenAIWSIngressHooks) []byte {
	if hooks == nil || !hooks.QuotaBypassEnabled {
		return payload
	}
	if injected, ok := InjectFunctionCallOutputSuffix(payload); ok {
		return injected
	}
	return payload
}

func shouldSkipOpenAIWSSessionPrewarm(payload []byte) bool {
	if strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String()) != "" {
		return true
	}
	var reqBody map[string]any
	if err := json.Unmarshal(payload, &reqBody); err != nil {
		return true
	}
	signals := AnalyzeToolContinuationSignals(reqBody)
	return signals.HasFunctionCallOutput || signals.HasToolCallContext || signals.HasItemReference
}

// prewarmOpenAIWSSession sends a generate=false request and drains its terminal
// event before the real client turn is written. This is deliberately separate
// from the downstream SSE keepalive: the upstream connection must observe a
// completed Responses turn before it can be reused as a warm session.
func (s *OpenAIGatewayService) prewarmOpenAIWSSession(
	ctx context.Context,
	lease *openAIWSConnLease,
	payload []byte,
	account *Account,
	hooks *OpenAIWSIngressHooks,
) error {
	if s == nil || lease == nil || account == nil || len(payload) == 0 {
		return nil
	}
	if lease.IsPrewarmed() || shouldSkipOpenAIWSSessionPrewarm(payload) {
		return nil
	}
	payload = quotaBypassPrewarmPayload(payload, hooks)
	if eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); eventType != "" && eventType != "response.create" {
		return nil
	}

	var warmup map[string]any
	if err := json.Unmarshal(payload, &warmup); err != nil {
		return fmt.Errorf("decode websocket prewarm payload: %w", err)
	}
	warmup["generate"] = false

	if err := lease.WriteJSONWithContextTimeout(ctx, warmup, s.openAIWSWriteTimeout()); err != nil {
		lease.MarkBroken()
		return fmt.Errorf("write websocket prewarm request: %w", err)
	}

	for {
		message, err := lease.ReadMessageWithContextTimeout(ctx, s.openAIWSReadTimeout())
		if err != nil {
			lease.MarkBroken()
			return fmt.Errorf("read websocket prewarm event: %w", err)
		}
		eventType, _, _ := parseOpenAIWSEventEnvelope(message)
		if eventType == "" {
			continue
		}
		if eventType == "error" {
			codeRaw, errTypeRaw, msgRaw := parseOpenAIWSErrorEventFields(message)
			s.persistOpenAIWSRateLimitSignal(ctx, account, lease.HandshakeHeaders(), message, codeRaw, errTypeRaw, msgRaw)
			if isOpenAIWSRateLimitError(codeRaw, errTypeRaw, msgRaw) {
				lease.MarkBroken()
				return &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), message...),
					ResponseHeaders: lease.HandshakeHeaders(),
				}
			}
			lease.MarkBroken()
			messageText := strings.TrimSpace(msgRaw)
			if messageText == "" {
				messageText = "upstream websocket prewarm failed"
			}
			return errors.New(messageText)
		}
		if !isOpenAIWSTerminalEvent(eventType) {
			continue
		}
		if eventType != "response.completed" && eventType != "response.done" {
			lease.MarkBroken()
			return fmt.Errorf("upstream websocket prewarm ended with %s", eventType)
		}
		lease.MarkPrewarmed()
		return nil
	}
}

// prewarmOpenAIWSPassthroughSession performs the same protocol warmup on the
// dedicated passthrough connection. It runs before the relay goroutines start,
// so warmup events are consumed locally and never reach the client.
func (s *OpenAIGatewayService) prewarmOpenAIWSPassthroughSession(
	ctx context.Context,
	conn openAIWSClientConn,
	payload []byte,
	account *Account,
	hooks *OpenAIWSIngressHooks,
	handshakeHeaders http.Header,
) error {
	if s == nil || conn == nil || account == nil || len(payload) == 0 || shouldSkipOpenAIWSSessionPrewarm(payload) {
		return nil
	}
	payload = quotaBypassPrewarmPayload(payload, hooks)
	if eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); eventType != "" && eventType != "response.create" {
		return nil
	}

	var warmup map[string]any
	if err := json.Unmarshal(payload, &warmup); err != nil {
		return fmt.Errorf("decode websocket passthrough prewarm payload: %w", err)
	}
	warmup["generate"] = false

	writeCtx, cancelWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	err := conn.WriteJSON(writeCtx, warmup)
	cancelWrite()
	if err != nil {
		return fmt.Errorf("write websocket passthrough prewarm request: %w", err)
	}

	for {
		readCtx, cancelRead := context.WithTimeout(ctx, s.openAIWSReadTimeout())
		message, readErr := conn.ReadMessage(readCtx)
		cancelRead()
		if readErr != nil {
			return fmt.Errorf("read websocket passthrough prewarm event: %w", readErr)
		}
		eventType, _, _ := parseOpenAIWSEventEnvelope(message)
		if eventType == "" {
			continue
		}
		if eventType == "error" {
			codeRaw, errTypeRaw, msgRaw := parseOpenAIWSErrorEventFields(message)
			s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, message, codeRaw, errTypeRaw, msgRaw)
			if isOpenAIWSRateLimitError(codeRaw, errTypeRaw, msgRaw) {
				return &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), message...),
					ResponseHeaders: cloneHeader(handshakeHeaders),
				}
			}
			messageText := strings.TrimSpace(msgRaw)
			if messageText == "" {
				messageText = "upstream websocket passthrough prewarm failed"
			}
			return errors.New(messageText)
		}
		if !isOpenAIWSTerminalEvent(eventType) {
			continue
		}
		if eventType != "response.completed" && eventType != "response.done" {
			return fmt.Errorf("upstream websocket passthrough prewarm ended with %s", eventType)
		}
		return nil
	}
}

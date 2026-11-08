package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestKiro uses the same pinned route for token refresh and inference.
func (s *GatewayService) requestKiro(ctx context.Context, account *Account, raw []byte, model, session string) (*http.Response, int, map[string]string, error) {
	if _, err := kiroProxy(account); err != nil {
		return nil, 0, nil, err
	}
	account, token, err := s.kiroAccessToken(ctx, account)
	if err != nil {
		return nil, 0, nil, err
	}
	proxy, err := kiroProxy(account)
	if err != nil {
		return nil, 0, nil, err
	}
	catalog, err := s.kiroModels(ctx, account, token)
	if err != nil {
		return nil, 0, nil, err
	}
	credentials := shallowCopyMap(account.Credentials)
	found := false
	for _, m := range catalog {
		if m.ID == kiro.ModelID(model) {
			credentials["kiro_request_fields"] = m.Schema.Properties
			found = true
			break
		}
	}
	if !found {
		return nil, 0, nil, infraerrors.BadRequest("KIRO_MODEL_UNAVAILABLE", "selected model is unavailable for this Kiro account")
	}
	body, converted, err := kiro.Convert(raw, model, session, credentials)
	if err != nil {
		return nil, 0, nil, infraerrors.BadRequest("KIRO_INVALID_REQUEST", err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiro.Endpoint(account.Credentials), bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, err
	}
	req = prepareKiroHTTPRequest(req, account.Credentials, token, kiro.OperationGeneration)
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	return resp, kiro.EstimateTokens(string(body)), converted.ToolNames, err
}

type kiroStreamBody struct {
	*io.PipeReader
	cancel   context.CancelFunc
	upstream io.ReadCloser
	refused  *atomic.Bool
}

func (b *kiroStreamBody) Close() error {
	b.cancel()
	_ = b.upstream.Close()
	return b.PipeReader.Close()
}

// Bridge native events into the existing OpenAI compatibility translators.
// Only the request goroutine accesses gin; the decoder owns the pipe and upstream body.
func (s *GatewayService) kiroAsAnthropic(ctx context.Context, c *gin.Context, a *Account, raw []byte, parsed *ParsedRequest, model string) (*http.Response, error) {
	value, _ := c.Get("api_key")
	key, _ := value.(*APIKey)
	if key == nil {
		return nil, errors.New("Kiro requires an authenticated API key scope")
	}
	session := ""
	if parsed != nil {
		session = s.GenerateSessionHash(parsed)
	}
	if session == "" {
		session = uuid.NewString()
	}
	groupID := derefGroupID(key.GroupID)
	if parsed != nil && parsed.GroupID != nil {
		groupID = derefGroupID(parsed.GroupID)
	}
	scope := fmt.Sprintf("%d:%d:%d:%d", key.UserID, key.ID, groupID, a.ID)
	if _, err := kiroProxy(a); err != nil {
		return nil, err
	}
	policy, err := s.kiroGroupCachePolicy(ctx, groupID)
	if err != nil {
		return nil, err
	}
	cache, cacheOK := s.cache.(KiroResponseCache)
	identity := sha256.Sum256([]byte(a.GetCredential("refresh_token") + a.GetCredential("api_key")))
	cacheKey, eligible := kiro.CacheKey(raw, fmt.Sprintf("%s:%s:%s:%s:%x", scope, model, kiro.EnvironmentKey(a.Credentials), a.GetCredential("profile_arn"), identity))
	useCache := cacheOK && eligible && policy.Enabled && policy.TargetPercent > 0
	c.Header("X-Sub2api-Cache", "bypass")
	if useCache {
		c.Header("X-Sub2api-Cache", "miss")
		cached, cacheErr := cache.GetKiroResponse(ctx, groupID, cacheKey, policy)
		var out kiro.Response
		if cacheErr == nil && len(cached) > 0 && json.Unmarshal(cached, &out) == nil && len(out.Content) > 0 {
			out.ID = "msg_" + uuid.NewString()
			kiroCacheRead(&out)
			var buffer bytes.Buffer
			err := kiro.Replay(&out, func(kind string, event any) error {
				data, err := json.Marshal(event)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(&buffer, "event: %s\ndata: %s\n\n", kind, data)
				return err
			})
			if err == nil {
				c.Header("X-Sub2api-Cache", "hit")
				c.Header("X-Sub2api-Usage-Source", "local_cache")
				if parsed != nil && parsed.OnUpstreamAccepted != nil {
					parsed.OnUpstreamAccepted()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{out.ID}}, Body: io.NopCloser(&buffer)}, nil
			}
		}
	}
	upstreamCtx, cancel := context.WithCancel(ctx)
	resp, input, toolNames, err := s.requestKiro(upstreamCtx, a, raw, model, kiro.StableID(scope, session))
	if err != nil {
		cancel()
		if infraerrors.IsBadRequest(err) {
			c.JSON(400, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
		}
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer cancel()
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, a, resp.StatusCode, resp.Header, data, model)
		}
		if s.shouldFailoverUpstreamError(resp.StatusCode) || resp.StatusCode == 402 {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: []byte(`{"error":{"message":"Kiro upstream rejected the request"}}`), ResponseHeaders: resp.Header.Clone()}
		}
		upstreamErr := kiro.GenerationResponseError(resp.StatusCode, data)
		c.JSON(resp.StatusCode, gin.H{"error": gin.H{"message": upstreamErr.Error()}})
		return nil, upstreamErr
	}
	if parsed != nil && parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}
	c.Header("X-Sub2api-Usage-Source", "estimated")
	reader, writer := io.Pipe()
	ready := make(chan error, 1)
	refused := &atomic.Bool{}
	go func() {
		defer cancel()
		defer func() { _ = resp.Body.Close() }()
		started := false
		emit := func(name string, event any) error {
			if name == "message_delta" {
				if fields, ok := event.(map[string]any); ok {
					if delta, ok := fields["delta"].(map[string]any); ok && delta["stop_reason"] == "refusal" {
						refused.Store(true)
					}
				}
			}
			if name == "message_stop" {
				return nil
			}
			b, e := json.Marshal(event)
			if e != nil {
				return e
			}
			if !started {
				// Decode starts on content or an explicit refusal terminal.
				started = true
				ready <- nil
			}
			_, e = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", name, b)
			return e
		}
		out, decodeErr := kiro.DecodeWithUsageFinalizer(resp.Body, model, input, emit, kiroCacheFinalizer(upstreamCtx, cache, groupID, cacheKey, policy, useCache), toolNames)
		if out != nil && out.StopReason == "refusal" {
			refused.Store(true)
		}
		if !started {
			ready <- decodeErr
			_ = writer.CloseWithError(decodeErr)
			return
		}
		if decodeErr == nil {
			_, decodeErr = fmt.Fprint(writer, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		}
		if decodeErr != nil {
			errorType, message := kiroStreamError(decodeErr)
			_ = emit("error", gin.H{"type": "error", "error": gin.H{"type": errorType, "message": message}})
		}
		_ = writer.CloseWithError(decodeErr)
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = reader.Close()
			return nil, err
		}
	case <-ctx.Done():
		cancel()
		_ = resp.Body.Close()
		_ = reader.Close()
		return nil, ctx.Err()
	}
	headers := resp.Header.Clone()
	headers.Set("X-Request-Id", "kiro_"+uuid.NewString())
	headers.Set("Content-Type", "text/event-stream")
	return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: &kiroStreamBody{PipeReader: reader, cancel: cancel, upstream: resp.Body, refused: refused}}, nil
}

func (s *AccountTestService) testKiroAccountConnection(c *gin.Context, a *Account, model, prompt string) error {
	if model == "" {
		model = "claude-haiku-4.5"
	}
	model = a.GetMappedModel(model)
	if prompt == "" {
		prompt = "Reply briefly with OK."
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model})
	gateway := &GatewayService{httpUpstream: s.httpUpstream, claudeTokenProvider: s.claudeTokenProvider}
	raw, _ := json.Marshal(map[string]any{"model": model, "max_tokens": 1024, "messages": []any{map[string]any{"role": "user", "content": prompt}}, "stream": true})
	resp, input, toolNames, err := gateway.requestKiro(c.Request.Context(), a, raw, model, kiro.StableID(fmt.Sprint(a.ID), uuid.NewString()))
	if err != nil {
		return s.sendErrorAndEnd(c, "Kiro test transport or credential refresh failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Kiro returned HTTP %d", resp.StatusCode))
	}
	out, err := kiro.Decode(resp.Body, model, input, func(kind string, event any) error {
		if kind == "content_block_delta" {
			m, _ := event.(map[string]any)
			delta, _ := m["delta"].(map[string]any)
			text, _ := delta["text"].(string)
			if text != "" {
				s.sendEvent(c, TestEvent{Type: "content", Text: text})
			}
		}
		return c.Request.Context().Err()
	}, toolNames)
	if err != nil {
		return s.sendErrorAndEnd(c, "Kiro test stream failed")
	}
	if out.StopReason == "refusal" {
		return s.sendErrorAndEnd(c, "Kiro refused the test request (CONTENT_FILTERED)")
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// Check controls before generic adapters can drop unsupported OpenAI fields.
func validateKiroOpenAIToolControls(raw []byte) error {
	var req struct {
		Choice   json.RawMessage `json:"tool_choice"`
		Parallel *bool           `json:"parallel_tool_calls"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return errors.New("invalid Kiro OpenAI request")
	}
	if req.Parallel != nil && !*req.Parallel {
		return errors.New("Kiro cannot enforce parallel_tool_calls=false")
	}
	if len(req.Choice) != 0 && string(req.Choice) != "null" {
		var choice string
		if json.Unmarshal(req.Choice, &choice) != nil || (choice != "auto" && choice != "none") {
			return errors.New("Kiro supports only auto or none tool_choice")
		}
	}
	return nil
}

// The pipe carries a request-local refusal flag; no mutable gin state crosses
// the decoder goroutine. Suppression applies to token and per-request pricing.
func markKiroRefusal(result *ForwardResult, body io.ReadCloser) *ForwardResult {
	if result != nil {
		if stream, ok := body.(*kiroStreamBody); ok && stream.refused != nil && stream.refused.Load() {
			result.KiroRefused = true
			result.Usage = ClaudeUsage{}
		}
	}
	return result
}

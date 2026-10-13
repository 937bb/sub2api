package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

const openAIProcessingRetryReason = "processing_error_fresh_connection"
const openAIProcessingPeekLimit = 64 * 1024

type processingRetryStateKey struct{}
type processingRetryState struct {
	used   atomic.Bool
	origin context.Context
}

func withProcessingRetryState(ctx context.Context) (context.Context, *processingRetryState) {
	if state, ok := ctx.Value(processingRetryStateKey{}).(*processingRetryState); ok {
		return ctx, state
	}
	state := &processingRetryState{origin: ctx}
	return context.WithValue(ctx, processingRetryStateKey{}, state), state
}

func claimOpenAIProcessingRetry(ctx context.Context) bool {
	_, state := withProcessingRetryState(ctx)
	return state.origin.Err() == nil && state.used.CompareAndSwap(false, true)
}

func openAIProcessingRetryUsed(ctx context.Context) bool {
	_, state := withProcessingRetryState(ctx)
	return state.used.Load()
}

func newOpenAIProcessingFailover(headers http.Header, payload []byte) *UpstreamFailoverError {
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseHeaders: headers.Clone(),
		ResponseBody: append([]byte(nil), payload...), RequestScopedTransient: true,
		Reason: GatewayFailureReason(openAIProcessingRetryReason), ClientStatusCode: http.StatusBadGateway,
		ClientMessage: sanitizeUpstreamErrorMessage(extractOpenAISSEErrorMessage(payload))}
}

// Re-enter the normal parser on a fresh transport only after it proves that
// no semantic output was committed. This preserves long-reasoning heartbeats.
func (s *OpenAIGatewayService) retryOpenAIProcessing(ctx context.Context, account *Account,
	attempt func(context.Context) (*OpenAIForwardResult, error)) (*OpenAIForwardResult, error) {
	ctx, state := withProcessingRetryState(ctx)
	start := time.Now()
	result, err := attempt(ctx)
	var failure *UpstreamFailoverError
	if !errors.As(err, &failure) || failure.Reason != GatewayFailureReason(openAIProcessingRetryReason) ||
		ctx.Err() != nil || isControlledExperiment(ctx) || account == nil || account.IsCopilotSDKEnabled() ||
		(s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account)) ||
		(result != nil && (result.Usage != (OpenAIUsage{}) || result.ImageCount > 0 || result.ClientDisconnect)) ||
		!state.used.CompareAndSwap(false, true) {
		return result, err
	}
	elapsed := time.Since(start)
	logger.LegacyPrintf("service.openai_gateway", "preoutput_error_reconnect account_id=%d transport=http_stream retry=1", account.ID)
	result, err = attempt(httpclient.WithFreshConnection(ctx))
	if result != nil {
		result.Duration += elapsed
		if result.FirstTokenMs != nil {
			value := *result.FirstTokenMs + int(elapsed.Milliseconds())
			result.FirstTokenMs = &value
		}
	}
	return result, err
}

// Inspect only explicit error fields. Prompt echoes and generated text cannot
// trigger a replay. A terminal response with output is not safe to replay.
func isOpenAIProcessingFailure(payload []byte) bool {
	if !openAIRetryPayloadSafe(payload) {
		return false
	}
	for _, path := range []string{"output", "response.output"} {
		output := gjson.GetBytes(payload, path)
		if output.Exists() && output.Raw != "null" && (!output.IsArray() || len(output.Array()) != 0) {
			return false
		}
	}
	for _, path := range []string{"error.message", "response.error.message"} {
		message := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, path).String()))
		if message == "an error occurred while processing your request" || strings.HasPrefix(message, "an error occurred while processing your request.") {
			return true
		}
	}
	return false
}

type openAIProcessingReplayBody struct {
	io.Reader
	io.Closer
}

type processingReadRecorder struct {
	io.Reader
	err error
}

func (r *processingReadRecorder) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

type processingReadError struct{ err error }

func (r *processingReadError) Read([]byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	err := r.err
	r.err = nil
	return 0, err
}

// Peek only bounded HTTP error bodies. SSE stays in the existing streaming
// parser so waiting for output cannot delay headers, heartbeats or cancellation.
func peekOpenAIProcessingFailure(resp *http.Response) bool {
	return peekOpenAIUpstreamRetryFailure(resp, false)
}

func peekOpenAIUpstreamRetryFailure(resp *http.Response, allowTemporary bool) bool {
	if resp == nil || resp.Body == nil || resp.StatusCode < 400 || isEventStreamResponse(resp.Header) {
		return false
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 529 {
		return false
	}
	original := resp.Body
	var captured bytes.Buffer
	recorded := &processingReadRecorder{Reader: original}
	reader := io.TeeReader(io.LimitReader(recorded, openAIProcessingPeekLimit), &captured)
	defer func() {
		resp.Body = &openAIProcessingReplayBody{Reader: io.MultiReader(bytes.NewReader(captured.Bytes()), &processingReadError{err: recorded.err}, original), Closer: original}
	}()
	body, err := io.ReadAll(reader)
	if err != nil || len(body) >= openAIProcessingPeekLimit {
		return false
	}
	if gjson.ValidBytes(body) {
		return isOpenAIProcessingFailure(body) || (allowTemporary && (isOpenAIPreOutputRetryFailure(body) || (openAIRetryHTTPStatus(resp.StatusCode) && openAIRetryPayloadSafe(body))))
	}
	return (allowTemporary && openAIRetryHTTPStatus(resp.StatusCode)) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(body))), "an error occurred while processing your request.")
}

func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if request == nil || account == nil {
		return nil, errors.New("upstream request and account are required")
	}
	ctx, state := withProcessingRetryState(request.Context())
	request = request.WithContext(ctx)
	resp, err := s.doOpenAIUpstreamAttempt(request, proxyURL, account)
	if err != nil || request == nil || account == nil || account.Platform != PlatformOpenAI ||
		isControlledExperiment(request.Context()) || request.Context().Err() != nil ||
		(request.Body != nil && request.Body != http.NoBody && request.GetBody == nil) ||
		(s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account)) {
		return resp, err
	}
	// API-key pools already own their configured status retry budget. Preserve
	// that budget instead of nesting a generic retry under every pool attempt.
	allowTemporary := openAIAccountAllowsTransientRetry(account) && request.URL != nil &&
		(strings.HasSuffix(request.URL.Path, "/responses") || strings.HasSuffix(request.URL.Path, "/chat/completions"))
	if !peekOpenAIUpstreamRetryFailure(resp, allowTemporary) || request.Context().Err() != nil || state.origin.Err() != nil || state.used.Load() {
		return resp, nil
	}
	// Do not shorten Retry-After. Longer waits belong to scheduler cooldowns.
	delay := time.Duration(0)
	if openAIRetryHTTPStatus(resp.StatusCode) {
		delay = 100 * time.Millisecond
	}
	if value := resp.Header.Get("Retry-After"); value != "" {
		if seconds, parseErr := strconv.Atoi(value); parseErr == nil {
			if seconds > 1 {
				return resp, nil
			}
			if seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
		} else if when, parseErr := http.ParseTime(value); parseErr == nil {
			remaining := time.Until(when)
			if remaining > time.Second {
				return resp, nil
			}
			if remaining > delay {
				delay = remaining
			}
		} else {
			return resp, nil
		}
	}
	retry := request.Clone(httpclient.WithFreshConnection(request.Context()))
	if request.GetBody != nil {
		body, bodyErr := request.GetBody()
		if bodyErr != nil {
			return resp, nil
		}
		retry.Body = body
	}
	if !state.used.CompareAndSwap(false, true) {
		if retry.Body != nil {
			_ = retry.Body.Close()
		}
		return resp, nil
	}
	_ = resp.Body.Close()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-state.origin.Done():
			if retry.Body != nil {
				_ = retry.Body.Close()
			}
			return nil, state.origin.Err()
		case <-timer.C:
		}
	}
	logger.LegacyPrintf("service.openai_gateway", "preoutput_error_reconnect account_id=%d transport=http retry=1 status=%d", account.ID, resp.StatusCode)
	// One additional attempt; exhausted errors go back to the normal handler.
	return s.doOpenAIUpstreamAttempt(retry, proxyURL, account)
}

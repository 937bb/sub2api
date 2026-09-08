package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

// The response body remains untouched on the final attempt. Only rejected HTTP
// responses are retried; successful streams and transport errors keep their path.
func retryOAuthHTTP(ctx context.Context, request *http.Request, settings OAuthRetrySettings, send func(*http.Request) (*http.Response, error)) (*http.Response, error, bool) {
	current := request
	for attempt := 0; ; attempt++ {
		resp, err := send(current)
		if err != nil {
			if settings.Enabled {
				slogOAuthRetry(request, "transport_error", 0, attempt, settings.MaxRetries, "transport")
			}
			return resp, err, false
		}
		if resp == nil || !settings.Enabled {
			return resp, err, false
		}
		matched := false
		for _, code := range settings.StatusCodes {
			if resp.StatusCode == code {
				matched = true
				break
			}
		}
		if !matched {
			if attempt > 0 {
				slogOAuthRetry(request, "response_received", resp.StatusCode, attempt, settings.MaxRetries, "status_not_configured")
			}
			return resp, nil, false
		}
		if attempt >= settings.MaxRetries {
			slogOAuthRetry(request, "exhausted", resp.StatusCode, attempt, settings.MaxRetries, "status_exhausted")
			return resp, nil, true
		}
		if request.GetBody == nil {
			slogOAuthRetry(request, "skipped", resp.StatusCode, attempt, settings.MaxRetries, "body_not_replayable")
			return resp, nil, true
		}
		replay, err := request.GetBody()
		if err != nil {
			slogOAuthRetry(request, "skipped", resp.StatusCode, attempt, settings.MaxRetries, "body_replay_failed")
			return resp, nil, true
		}
		timer := time.NewTimer(oauthRetryDelay(attempt, resp.Header.Get("Retry-After"), time.Now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			replay.Close()
			slogOAuthRetry(request, "cancelled", resp.StatusCode, attempt, settings.MaxRetries, "context_cancelled")
			return resp, nil, true
		case <-timer.C:
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		slogOAuthRetry(request, "retry", resp.StatusCode, attempt+1, settings.MaxRetries, "configured_status")
		current = request.Clone(request.Context())
		current.Body = replay
	}
}

func oauthRetryDelay(attempt int, retryAfter string, now time.Time) time.Duration {
	delay := 100 * time.Millisecond
	for i := 0; i < attempt && delay < 800*time.Millisecond; i++ {
		delay *= 2
	}
	if seconds, err := strconv.ParseInt(retryAfter, 10, 32); err == nil && seconds > 0 {
		delay = time.Duration(seconds) * time.Second
	} else if when, err := http.ParseTime(retryAfter); err == nil && when.After(now) {
		delay = when.Sub(now)
	}
	if delay < 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	if delay > 800*time.Millisecond {
		return 800 * time.Millisecond
	}
	return delay
}

func (s *OpenAIGatewayService) doOAuthResponsesUpstream(c *gin.Context, request *http.Request, proxyURL string, account *Account) (*http.Response, error, bool) {
	send := func(r *http.Request) (*http.Response, error) { return s.doOpenAIUpstream(r, proxyURL, account) }
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || c == nil || c.Request == nil || c.Writer.Written() || IsResponseCommitted(c) {
		resp, err := send(request)
		return resp, err, false
	}
	settings, err := s.settingService.GetOAuthRetrySettings(c.Request.Context())
	if err != nil {
		settings = DefaultOAuthRetrySettings()
	}
	if !settings.Enabled {
		resp, err := send(request)
		return resp, err, false
	}
	upstreamCtx, cancel := context.WithCancel(request.Context())
	upstreamCtx = withOAuthRetryLogContext(upstreamCtx, c.Request.Context(), account.ID)
	stop := context.AfterFunc(c.Request.Context(), cancel)
	cleanup := func() { stop(); cancel() }
	resp, sendErr, exhausted := retryOAuthHTTP(c.Request.Context(), request.Clone(upstreamCtx), settings, send)
	if resp != nil && resp.Body != nil {
		resp.Body = &openAIRequestContextReadCloser{ReadCloser: resp.Body, cleanup: cleanup}
	} else {
		cleanup()
	}
	return resp, sendErr, exhausted
}

// Exhausted configured statuses bypass further failover retries and error mapping.
// Header filtering preserves the existing gateway boundary for upstream headers.
func writeOAuthRetryExhausted(c *gin.Context, resp *http.Response) error {
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
	defer resp.Body.Close()
	setOpsUpstreamError(c, resp.StatusCode, "OAuth upstream retries exhausted", "")
	for name, values := range responseheaders.FilterHeaders(resp.Header, nil) {
		c.Writer.Header()[name] = append([]string(nil), values...)
	}
	MarkResponseCommitted(c)
	c.Status(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		return fmt.Errorf("oauth retry exhausted response: %w", err)
	}
	return fmt.Errorf("oauth upstream retries exhausted: HTTP %d", resp.StatusCode)
}

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const oauthMappedRetryKey = "oauth_mapped_retry_active"

func oauthMappedRetryActive(c *gin.Context) bool { return c != nil && c.GetBool(oauthMappedRetryKey) }

func (s *OpenAIGatewayService) forwardWithOAuthMappedRetry(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || c == nil || c.Request == nil || c.Writer.Written() || IsResponseCommitted(c) || oauthMappedRetryActive(c) {
		return s.forwardOnce(ctx, c, account, body)
	}
	settings, err := s.settingService.GetOAuthRetrySettings(ctx)
	if err != nil || !settings.Enabled {
		return s.forwardOnce(ctx, c, account, body)
	}
	return runOAuthMappedRetry(ctx, c, account.ID, body, settings, func(attemptBody []byte) (*OpenAIForwardResult, error) {
		return s.forwardOnce(ctx, c, account, attemptBody)
	})
}

func runOAuthMappedRetry(ctx context.Context, c *gin.Context, accountID int64, body []byte, settings OAuthRetrySettings, forward func([]byte) (*OpenAIForwardResult, error)) (*OpenAIForwardResult, error) {
	originalWriter := c.Writer
	defer func() { c.Writer = originalWriter; c.Set(oauthMappedRetryKey, false) }()
	keys := make(map[string]any, len(c.Keys))
	for _, k := range []string{ResponseCommittedKey, OpsStreamErrorKey, OpsStreamErrorsKey, OpsSkipPassthroughKey} {
		if v, ok := c.Get(k); ok {
			keys[k] = v
		}
	}
	headers := originalWriter.Header().Clone()
	canonical := append([]byte(nil), body...)
	logRequest := c.Request.Clone(withOAuthRetryLogContext(ctx, c.Request.Context(), accountID))
	for attempt := 0; ; attempt++ {
		c.Set(oauthMappedRetryKey, true)
		c.Set("oauth_mapped_retry_settings", settings)
		writer := &oauthRetryWriter{ResponseWriter: originalWriter}
		c.Writer = writer
		result, forwardErr := forward(append([]byte(nil), canonical...))
		status := oauthMappedFailureStatus(writer, forwardErr)
		matched := false
		for _, code := range settings.StatusCodes {
			if code == status {
				matched = true
				break
			}
		}
		if !matched || (forwardErr == nil && writer.status < 400) {
			if attempt > 0 {
				slogOAuthRetry(logRequest, "response_received", writer.Status(), attempt, settings.MaxRetries, "forward_finished")
			}
			if err := writer.commit(); err != nil && forwardErr == nil {
				forwardErr = err
			}
			return result, forwardErr
		}
		if originalWriter.Written() || result != nil || GetOpsCyberPolicy(c) != nil || c.GetBool(OpsClientBusinessLimitedKey) || ctx.Err() != nil || c.Request.Context().Err() != nil {
			reason := "response_already_written"
			if result != nil {
				reason = "billable_result_present"
			}
			if ctx.Err() != nil || c.Request.Context().Err() != nil {
				reason = "context_cancelled"
			}
			slogOAuthRetry(logRequest, "skipped", status, attempt, settings.MaxRetries, reason)
			_ = writer.commit()
			return result, forwardErr
		}
		if attempt >= settings.MaxRetries {
			slogOAuthRetry(logRequest, "exhausted", status, attempt, settings.MaxRetries, "mapped_status_exhausted")
			if err := writer.commit(); err != nil {
				return nil, err
			}
			var failover *UpstreamFailoverError
			if errors.As(forwardErr, &failover) {
				copied := *failover
				copied.NextAccountAction = NextAccountStop
				copied.RetryableOnSameAccount = false
				forwardErr = &copied
			}
			if forwardErr == nil {
				forwardErr = fmt.Errorf("oauth mapped retry exhausted: HTTP %d", status)
			}
			return nil, forwardErr
		}
		timer := time.NewTimer(oauthRetryDelay(attempt, writer.Header().Get("Retry-After"), time.Now()))
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			_ = writer.commit()
			return nil, c.Request.Context().Err()
		case <-ctx.Done():
			timer.Stop()
			_ = writer.commit()
			return nil, ctx.Err()
		case <-timer.C:
		}
		slogOAuthRetry(logRequest, "retry", status, attempt+1, settings.MaxRetries, "mapped_status")
		// Discard only the failed attempt's uncommitted response and request-local markers.
		for k := range originalWriter.Header() {
			delete(originalWriter.Header(), k)
		}
		for k, v := range headers {
			originalWriter.Header()[k] = append([]string(nil), v...)
		}
		for _, k := range []string{ResponseCommittedKey, OpsStreamErrorKey, OpsStreamErrorsKey, OpsSkipPassthroughKey} {
			if v, ok := keys[k]; ok {
				c.Set(k, v)
			} else {
				delete(c.Keys, k)
			}
		}
		SetOpenAIQuotaBypassEnabled(c, c.GetBool(openAIQuotaBypassEnabledContextKey))
	}
}

func oauthMappedFailureStatus(w *oauthRetryWriter, err error) int {
	if w.status >= 400 {
		return w.status
	}
	if err == nil {
		return 0
	}
	var failover *UpstreamFailoverError
	if errors.As(err, &failover) {
		if failover.ClientStatusCode >= 400 {
			return failover.ClientStatusCode
		}
		if failover.IsCredentialFailure() {
			return http.StatusServiceUnavailable
		}
		if failover.StatusCode == 429 {
			return 429
		}
		if failover.StatusCode == 529 || (failover.StatusCode == 503 && failover.RequestScopedTransient) {
			return 503
		}
	}
	return http.StatusBadGateway
}

func (s *OpenAIGatewayService) newOAuthMappedStreamError(c *gin.Context, account *Account, payload []byte, message string, headers http.Header) error {
	s.recordOpenAIStreamUpstreamError(c, account, c.GetBool("openai_passthrough"), headers.Get("x-request-id"), "failover", payload, message)
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: openAIStreamFailedEventPassthroughBody(payload, message), ResponseHeaders: headers.Clone()}
}

func (s *OpenAIGatewayService) shouldRetryOAuthMappedStream(c *gin.Context, account *Account, payload []byte, message string) bool {
	if !oauthMappedRetryActive(c) {
		return false
	}
	v, _ := c.Get("oauth_mapped_retry_settings")
	settings, ok := v.(OAuthRetrySettings)
	if !ok {
		return false
	}
	matched := false
	for _, code := range settings.StatusCodes {
		if code == 502 {
			matched = true
		}
	}
	if !matched || s.openAIStreamFailureStatusForRequest(c, payload, message) != 502 {
		return false
	}
	if status, _, _, matched := applyOpenAIStreamFailedErrorPassthroughRule(c, account.Platform, payload, message); matched && status != 502 {
		return false
	}
	return true
}

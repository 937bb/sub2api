package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

func openAICodexHTTPProfile(account *Account) *tlsfingerprint.Profile {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return nil
	}
	return tlsfingerprint.CodexHTTPProfile()
}

// doOpenAIUpstream resolves the account route once for WS, plugin and HTTP.
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if attempt, _ := request.Context().Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt); attempt != nil && attempt.sent.Load() {
		return nil, finishGroupSmartWSAttempt(attempt, errSmartWSAlreadyDispatched)
	}
	return doWithAccountProxyFallback(request, proxyURL, account, func(req *http.Request, route string) (response *http.Response, resultErr error) {
		defer func() { auditOpenAIHTTPResult(req, account, response, resultErr) }()
		auditOpenAIHTTPRequest(req, account)
		if response, handled, err := s.tryGroupSmartWS(req, route, account); handled {
			return response, err
		}
		if s.pluginManager != nil {
			response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(req.Context(), req, route, account)
			if handled {
				return response, err
			}
		}
		if profile := openAICodexHTTPProfile(account); profile != nil {
			return s.httpUpstream.DoWithTLS(req, route, account.ID, account.Concurrency, profile)
		}
		return s.httpUpstream.Do(req, route, account.ID, account.Concurrency)
	})
}

// Account tests use the same validated route as production forwarding.
func (s *AccountTestService) doOpenAIAccountTestUpstream(request *http.Request, proxyURL string, account *Account, useTLSFallback bool) (*http.Response, error) {
	return doWithAccountProxyFallback(request, proxyURL, account, func(req *http.Request, route string) (response *http.Response, resultErr error) {
		defer func() { auditOpenAIHTTPResult(req, account, response, resultErr) }()
		auditOpenAIHTTPRequest(req, account)
		if s.pluginManager != nil {
			response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(req.Context(), req, route, account)
			if handled {
				return response, err
			}
		}
		if profile := openAICodexHTTPProfile(account); profile != nil {
			return s.httpUpstream.DoWithTLS(req, route, account.ID, account.Concurrency, profile)
		}
		if useTLSFallback && s.tlsFPProfileService != nil {
			return s.httpUpstream.DoWithTLS(req, route, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
		}
		return s.httpUpstream.Do(req, route, account.ID, account.Concurrency)
	})
}

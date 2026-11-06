package service

import "net/http"

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream resolves the account route once for WS, plugin and HTTP.
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if attempt, _ := request.Context().Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt); attempt != nil && attempt.sent.Load() {
		return nil, finishGroupSmartWSAttempt(attempt, errSmartWSAlreadyDispatched)
	}
	return doWithAccountProxyFallback(request, proxyURL, account, func(req *http.Request, route string) (*http.Response, error) {
		if response, handled, err := s.tryGroupSmartWS(req, route, account); handled {
			return response, err
		}
		if s.pluginManager != nil {
			response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(req.Context(), req, route, account)
			if handled {
				return response, err
			}
		}
		return s.httpUpstream.Do(req, route, account.ID, account.Concurrency)
	})
}

// Account tests use the same validated route as production forwarding.
func (s *AccountTestService) doOpenAIAccountTestUpstream(request *http.Request, proxyURL string, account *Account, useTLSFallback bool) (*http.Response, error) {
	return doWithAccountProxyFallback(request, proxyURL, account, func(req *http.Request, route string) (*http.Response, error) {
		if s.pluginManager != nil {
			response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(req.Context(), req, route, account)
			if handled {
				return response, err
			}
		}
		if useTLSFallback {
			return s.httpUpstream.DoWithTLS(req, route, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
		}
		return s.httpUpstream.Do(req, route, account.ID, account.Concurrency)
	})
}

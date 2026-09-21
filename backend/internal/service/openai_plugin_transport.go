package service

import "net/http"

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream delegates live requests to a plugin only when the OpenAI
// OAuth capability binding is enabled. Response parsing, error mapping, SSE,
// and billing remain in the core pipeline.
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	normalizeCodexResponsesTransportHeaders(request, account)
	proxyURL = s.openAICodexTurnStateRouteProxyURL(account, request.Header, proxyURL)
	profile, err := resolveMode1TLSProfile(account)
	if err != nil {
		return nil, err
	}
	if profile != nil && (s.cfg == nil || s.cfg.Gateway.TLSFingerprint.Enabled) && (s.pluginManager == nil || !s.pluginManager.ShouldRouteOpenAIOAuth(account)) {
		return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Mode1EffectiveConcurrency(), profile)
	}
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Mode1EffectiveConcurrency())
}

// doOpenAIAccountTestUpstream keeps OAuth account tests on the same plugin
// path as live forwarding. API keys and accounts without a plugin binding keep
// their existing HTTPUpstream behavior.
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	normalizeCodexResponsesTransportHeaders(request, account)
	if s.openaiGatewayService != nil {
		proxyURL = s.openaiGatewayService.openAICodexTurnStateRouteProxyURL(account, request.Header, proxyURL)
	}
	profile, err := resolveMode1TLSProfile(account)
	if err != nil {
		return nil, err
	}
	if profile != nil && (s.cfg == nil || s.cfg.Gateway.TLSFingerprint.Enabled) && (s.pluginManager == nil || !s.pluginManager.ShouldRouteOpenAIOAuth(account)) {
		return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Mode1EffectiveConcurrency(), profile)
	}
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback && !isMode1ProtectionEnabled(account) {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Mode1EffectiveConcurrency())
}

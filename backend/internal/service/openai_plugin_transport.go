package service

import (
	"errors"
	"net/http"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstreamAttempt retains ticket binding and plugin routing for each attempt.
func (s *OpenAIGatewayService) doOpenAIUpstreamAttempt(request *http.Request, proxyURL string, account *Account) (result *http.Response, resultErr error) {
	if account.IsOpenAISiwc() {
		if account.ProxyID != nil && proxyURL == "" {
			return nil, errors.New("SIWC configured proxy unavailable")
		}
		target := runtimeProxyEgress{url: proxyURL}
		if account.ProxyID != nil {
			target.proxyID = *account.ProxyID
		}
		return s.doOpenAIProxyAttempt(request, account, target)
	}
	request = s.stickBoundCodexTicketRequest(request, account)
	boundTicket := s.codexTicketRequestBound(request, account)
	releaseChat := func() {}
	if boundTicket {
		releaseChat = s.holdCodexTicketChat(account)
	}
	proxyURL, release, err := s.pinCodexTicketEgress(request, account, proxyURL)
	if err != nil {
		releaseChat()
		return nil, err
	}
	defer func() {
		result, resultErr = attachCodexTicketEgressRelease(result, resultErr, func() {
			release()
			releaseChat()
		})
	}()
	defer func() {
		if resultErr == nil {
			s.observeCodexTicketResponse(request, result, account)
			if boundTicket && codexResponseMismatches(request, result, account) && s.settingService.CodexTicketStrictResponse(request.Context()) {
				if result.Body != nil {
					_ = result.Body.Close()
				}
				result = nil
				resultErr = ErrCodexTicketResponseRejected
			}
		}
	}()
	// Keep ticket observation/strict validation around the final response,
	// while every egress attempt retains its own plugin routing and trace.
	return s.doUpstreamWithProxyFallback(request.Context(), request, account, proxyURL)
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

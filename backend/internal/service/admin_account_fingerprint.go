package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"os"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// AccountFingerprint is an allowlist of identity diagnostics. It never includes
// bearer credentials, fingerprint seeds, or request-dependent session IDs.
type AccountFingerprint struct {
	AccountID           int64                         `json:"account_id"`
	CredentialAccountID int64                         `json:"credential_account_id"`
	Inherited           bool                          `json:"inherited"`
	InstallationID      string                        `json:"installation_id"`
	Source              string                        `json:"source"`
	ConvergenceMode     string                        `json:"convergence_mode"`
	ConvergenceReady    bool                          `json:"convergence_ready"`
	UserAgentOverride   string                        `json:"user_agent_override"`
	WSAccountMode       string                        `json:"ws_account_mode"`
	Runtime             *AccountFingerprintRuntime    `json:"runtime,omitempty"`
	Transports          []AccountFingerprintTransport `json:"transports,omitempty"`
}

func (s *adminServiceImpl) GetAccountFingerprint(ctx context.Context, id int64) (*AccountFingerprint, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !account.IsOpenAIOAuthLike() {
		return nil, infraerrors.BadRequest("FINGERPRINT_UNSUPPORTED", "Fingerprint diagnostics require an OpenAI OAuth or setup-token account")
	}
	owner, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, err
	}
	result := accountFingerprintSummary(account, owner)
	identity := resolveCodexOutboundIdentity(account.GetOpenAIUserAgent())
	runtime := &AccountFingerprintRuntime{
		Scope: "serving_admin_process", UserAgent: identity.userAgent, Originator: identity.originator, Version: identity.version,
		IdentityEnforced: codexIdentityEnforcement.Load(), HTTPProfile: tlsfingerprint.CodexHTTPProfile().Name,
		LocalCLITransport: openai.IsLocalCLIUserAgent(identity.userAgent), CaptureStatus: "not_captured",
		DynamicHeaders: []string{"authorization", "chatgpt-account-id", "session_id", "conversation_id", "session-id", "thread-id", "x-client-request-id", "x-codex-turn-metadata", "x-codex-window-id"},
		CustomCA:       strings.TrimSpace(os.Getenv("CODEX_CA_CERTIFICATE")) != "" || strings.TrimSpace(os.Getenv("SSL_CERT_FILE")) != "",
	}
	if s.cfg != nil {
		runtime.StrictConversationBinding = s.cfg.Gateway.OpenAIConversationBindingEnabled
		runtime.ForceHTTP = s.cfg.Gateway.OpenAIWS.ForceHTTP
		runtime.WSRouterEnabled = s.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled
	}
	result.Runtime = runtime
	for _, profile := range []*tlsfingerprint.Profile{tlsfingerprint.CodexLocalHTTPProfile(), tlsfingerprint.CodexCustomCAHTTPProfile(), tlsfingerprint.CodexLocalWSProfile()} {
		transport := AccountFingerprintTransport{Profile: profile, ConfigurationHash: profile.CacheKey(), Evidence: "reference_capture_2026-11-03"}
		switch profile.Name {
		case tlsfingerprint.CodexLocalHTTPProfile().Name:
			transport.Protocol = "HTTP/1.1"
			transport.HeaderOrder = []string{"Host", "User-Agent", "Content-Length", "remaining headers sorted"}
			transport.SelectedByCurrentHTTP = runtime.LocalCLITransport && runtime.HTTPProfile == profile.Name
		case tlsfingerprint.CodexCustomCAHTTPProfile().Name:
			transport.Protocol = "ALPN: h2 or http/1.1"
			transport.HeaderOrder = append(tlsfingerprint.CodexRustlsHTTPHeaderOrder(), "remaining headers sorted")
			transport.HTTP2 = &AccountFingerprintHTTP2{Settings: tlsfingerprint.CodexHTTP2Settings(), ConnectionWindowUpdate: tlsfingerprint.CodexHTTP2ConnectionWindowUpdate, PseudoHeaderOrder: tlsfingerprint.CodexHTTP2PseudoHeaderOrder()}
			transport.SelectedByCurrentHTTP = runtime.LocalCLITransport && runtime.HTTPProfile == profile.Name
		default:
			transport.Protocol = "WebSocket over HTTP/1.1"
			transport.HeaderOrder = append(codexWSHeaderOrder(), "remaining headers sorted", "sec-websocket-extensions")
			transport.Compression = "permessage-deflate; context takeover; client_max_window_bits offer; negotiated with upstream"
		}
		result.Transports = append(result.Transports, transport)
	}
	return result, nil
}

func accountFingerprintSummary(account, owner *Account) *AccountFingerprint {
	installationID := codexAccountInstallationID(owner)
	source := "unavailable"
	if installationID != "" {
		if owner.GetOpenAIDeviceID() != "" {
			source = "explicit_device"
		} else if _, ok := codexFingerprintSeed(owner.Extra); ok {
			source = "persisted_seed"
		} else if strings.TrimSpace(owner.GetChatGPTAccountID()) != "" {
			source = "account_metadata"
		} else {
			source = "setup_token"
		}
	}
	mode := account.GetCodexFingerprintMode()
	_, hasSeed := codexFingerprintSeed(account.Extra)
	// Different defaults expose whether an account-level override actually exists,
	// without pretending the admin HTTP process knows the separate WS runtime.
	wsMode := account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff)
	if wsMode != account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool) {
		wsMode = "default"
	}
	return &AccountFingerprint{
		AccountID: account.ID, CredentialAccountID: owner.ID, Inherited: account.IsShadow(),
		InstallationID: installationID, Source: source, ConvergenceMode: string(mode),
		ConvergenceReady:  mode == codexFingerprintOff || hasSeed,
		UserAgentOverride: account.GetOpenAIUserAgent(), WSAccountMode: wsMode,
	}
}

// Runtime describes only this process, never an unobserved separate WS runtime.
type AccountFingerprintRuntime struct {
	Scope                     string   `json:"scope"`
	UserAgent                 string   `json:"user_agent"`
	Originator                string   `json:"originator"`
	Version                   string   `json:"version"`
	IdentityEnforced          bool     `json:"identity_enforced"`
	LocalCLITransport         bool     `json:"local_cli_transport"`
	HTTPProfile               string   `json:"http_profile"`
	CustomCA                  bool     `json:"custom_ca"`
	ForceHTTP                 bool     `json:"force_http"`
	WSRouterEnabled           bool     `json:"ws_router_enabled"`
	StrictConversationBinding bool     `json:"strict_conversation_binding"`
	CaptureStatus             string   `json:"capture_status"`
	DynamicHeaders            []string `json:"dynamic_headers"`
}
type AccountFingerprintTransport struct {
	Profile               *tlsfingerprint.Profile  `json:"tls"`
	ConfigurationHash     string                   `json:"configuration_hash"`
	Evidence              string                   `json:"evidence"`
	Protocol              string                   `json:"protocol"`
	SelectedByCurrentHTTP bool                     `json:"selected_by_current_http"`
	HeaderOrder           []string                 `json:"header_order"`
	HTTP2                 *AccountFingerprintHTTP2 `json:"http2,omitempty"`
	Compression           string                   `json:"compression,omitempty"`
}
type AccountFingerprintHTTP2 struct {
	Settings               [][2]uint32 `json:"settings"`
	ConnectionWindowUpdate uint32      `json:"connection_window_update"`
	PseudoHeaderOrder      []string    `json:"pseudo_header_order"`
}

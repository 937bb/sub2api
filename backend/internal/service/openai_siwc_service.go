package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
)

type OpenAISiwcCredential = siwc.Credential

type openAISiwcSession struct {
	mu         sync.Mutex
	protocol   *siwc.Session
	proxyID    *int64
	proxyURL   string
	credential *siwc.Credential
	models     []string
	account    *Account
	claimed    bool
	accountID  int64
	timer      *time.Timer
}

func (a *Account) IsOpenAISiwc() bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth {
		return false
	}
	protocol, _ := a.Extra["auth_protocol"].(string)
	// Legacy imports only stored a display label or issued client ID. Detect
	// these to fail closed instead of sending sharing tokens to Codex.
	return strings.EqualFold(a.GetCredential("auth_mode"), siwc.AuthMode) ||
		strings.EqualFold(protocol, siwc.AuthMode) || strings.HasPrefix(a.GetCredential("client_id"), "oaiapp_")
}

func validateSIWCCreation(account *Account) error {
	if account.IsOpenAISiwc() && (account.GetCredential("auth_mode") != siwc.AuthMode ||
		account.GetCredential("subject") == "" || !siwc.HasSharingScopes(account.GetCredential("granted_scope"))) {
		return siwcBadRequest(errors.New("Use OpenAI OAuth > SIWC authorization; a SIWC label alone is not an authorization"))
	}
	return nil
}

func (s *OpenAIOAuthService) RecoverLegacySIWC(ctx context.Context, account *Account) (map[string]any, error) {
	if !account.IsOpenAISiwc() || strings.EqualFold(account.GetCredential("auth_mode"), siwc.AuthMode) {
		return nil, siwcBadRequest(errors.New("SIWC legacy account required"))
	}
	proxyURL, err := s.siwcProxy(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	client, err := newSIWCClient(proxyURL)
	if err != nil {
		return nil, err
	}
	credential, err := client.RecoverLegacy(ctx, siwcCredentialFromAccount(account))
	if err != nil {
		return nil, siwcBadRequest(err)
	}
	credentials := openAISiwcCredentials(*credential)
	credentials["siwc_models"] = []string{}
	if credential.ExpiresAt > time.Now().Unix() {
		models, err := client.Models(ctx, credential.AccessToken)
		if err != nil {
			return nil, siwcBadRequest(err)
		}
		credentials["siwc_models"] = models
	}
	return credentials, nil
}

// SIWC is an independent OAuth grant; its token must never reach Codex endpoints.
func openAISiwcCredentials(c siwc.Credential) map[string]any {
	result := map[string]any{"auth_mode": siwc.AuthMode, "issuer": siwc.Issuer,
		"access_token": c.AccessToken, "id_token": c.IDToken, "client_id": c.ClientID,
		"ext_agent_host_id": c.HostID, "subject": c.Subject, "email": c.Email,
		"granted_scope": c.Scope, "token_type": "Bearer", "expires_at": time.Unix(c.ExpiresAt, 0).UTC().Format(time.RFC3339),
		"earliest_refresh_at": c.EarliestRefreshAt}
	if c.RefreshToken != "" {
		result["refresh_token"] = c.RefreshToken
	}
	return result
}

func siwcCredentialFromAccount(a *Account) siwc.Credential {
	c := siwc.Credential{AccessToken: a.GetCredential("access_token"), RefreshToken: a.GetCredential("refresh_token"),
		IDToken: a.GetCredential("id_token"), ClientID: a.GetCredential("client_id"), HostID: a.GetCredential("ext_agent_host_id"),
		Subject: a.GetCredential("subject"), Email: a.GetCredential("email"), Scope: a.GetCredential("granted_scope")}
	if expires := a.GetCredentialAsTime("expires_at"); expires != nil {
		c.ExpiresAt = expires.Unix()
	}
	switch v := a.Credentials["earliest_refresh_at"].(type) {
	case int64:
		c.EarliestRefreshAt = v
	case float64:
		c.EarliestRefreshAt = int64(v)
	case int:
		c.EarliestRefreshAt = int64(v)
	}
	return c
}

func (s *OpenAIOAuthService) siwcProxy(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", errors.New("SIWC proxy repository unavailable")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil || proxy == nil {
		return "", errors.New("SIWC configured proxy unavailable")
	}
	return proxy.URL(), nil
}

func newSIWCClient(proxyURL string) (*siwc.Client, error) {
	client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL})
	if err != nil {
		return nil, errors.New("SIWC proxy configuration invalid")
	}
	return siwc.NewClient(client), nil
}

func siwcBadRequest(err error) error {
	return infraerrors.New(http.StatusBadRequest, "OPENAI_SIWC_FAILED", err.Error())
}

func (s *OpenAIOAuthService) GenerateSIWCAuthURL(ctx context.Context, proxyID *int64, hostID string, resumeSession ...string) (*OpenAIAuthURLResult, error) {
	var registration *siwc.Registration
	if len(resumeSession) > 0 && resumeSession[0] != "" {
		var err error
		registration, err = siwc.LoadRegistration(s.siwcStateDir, resumeSession[0])
		if err != nil || registration.AccountID != 0 {
			return nil, siwcBadRequest(errors.New("SIWC registration cannot be resumed"))
		}
		proxyID, hostID = registration.ProxyID, registration.HostID
	}
	return s.generateSIWCAuthURL(ctx, proxyID, hostID, nil, registration)
}

func (s *OpenAIOAuthService) GenerateSIWCReauthURL(ctx context.Context, account *Account) (*OpenAIAuthURLResult, error) {
	if !account.IsOpenAISiwc() {
		return nil, siwcBadRequest(errors.New("SIWC account required"))
	}
	return s.generateSIWCAuthURL(ctx, account.ProxyID, account.GetCredential("ext_agent_host_id"), account, nil)
}

func (s *OpenAIOAuthService) generateSIWCAuthURL(ctx context.Context, proxyID *int64, hostID string, account *Account, registration *siwc.Registration) (*OpenAIAuthURLResult, error) {
	proxyURL, err := s.siwcProxy(ctx, proxyID)
	if err != nil {
		return nil, siwcBadRequest(err)
	}
	if (account == nil && registration == nil) || hostID == "" {
		hostID, err = siwc.StableHostID(s.siwcStateDir, hostID)
		if err != nil {
			return nil, siwcBadRequest(err)
		}
	}
	protocol, err := siwc.NewSession(hostID)
	if err != nil {
		return nil, siwcBadRequest(err)
	}
	var accountID int64
	if account != nil {
		protocol.ClientID, protocol.Subject = account.GetCredential("client_id"), account.GetCredential("subject")
		protocol.IDTokenHint, protocol.LoginHint = account.GetCredential("id_token"), account.GetCredential("email")
		if protocol.ClientID == "" || protocol.Subject == "" {
			return nil, siwcBadRequest(errors.New("SIWC identity incomplete"))
		}
		accountID = account.ID
	} else if registration != nil {
		protocol.ClientID = registration.ClientID
	}
	id, err := openai.GenerateSessionID()
	if err != nil {
		return nil, err
	}
	s.siwcMu.Lock()
	defer s.siwcMu.Unlock()
	if s.siwcSessions == nil {
		s.siwcSessions = make(map[string]*openAISiwcSession)
	}
	for key, flow := range s.siwcSessions {
		if time.Since(flow.protocol.CreatedAt) > siwc.SessionTTL {
			delete(s.siwcSessions, key)
		}
	}
	if len(s.siwcSessions) >= 256 {
		return nil, siwcBadRequest(errors.New("too many pending SIWC authorizations"))
	}
	if err := siwc.SaveRegistration(s.siwcStateDir, id, siwc.Registration{ClientID: protocol.ClientID,
		HostID: protocol.HostID, AccountID: accountID, ProxyID: proxyID, CreatedAt: time.Now()}); err != nil {
		return nil, siwcBadRequest(errors.New("cannot persist SIWC registration"))
	}
	flow := &openAISiwcSession{protocol: protocol, proxyID: proxyID, proxyURL: proxyURL, accountID: accountID}
	s.siwcSessions[id] = flow
	flow.timer = time.AfterFunc(siwc.SessionTTL, func() {
		s.siwcMu.Lock()
		delete(s.siwcSessions, id)
		s.siwcMu.Unlock()
	})
	return &OpenAIAuthURLResult{AuthURL: protocol.AuthorizationURL(), SessionID: id, HostID: protocol.HostID}, nil
}

// CompleteSIWC retains exchanged credentials on database errors and serializes
// completion. A retry returns the original account instead of creating a duplicate.
func (s *OpenAIOAuthService) CompleteSIWC(ctx context.Context, id, callback string, accountID int64, create func(map[string]any, *int64) (*Account, error)) (*Account, error) {
	s.siwcMu.Lock()
	flow := s.siwcSessions[id]
	s.siwcMu.Unlock()
	if flow == nil {
		return nil, siwcBadRequest(errors.New("SIWC session missing or expired"))
	}
	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.accountID != accountID {
		return nil, siwcBadRequest(errors.New("SIWC session account mismatch"))
	}
	code, clientID, err := flow.protocol.Callback(strings.TrimSpace(callback))
	if err != nil {
		return nil, siwcBadRequest(err)
	}
	if flow.account != nil {
		return flow.account, nil
	}
	client, err := newSIWCClient(flow.proxyURL)
	if err != nil {
		return nil, siwcBadRequest(err)
	}
	if flow.credential == nil {
		if flow.claimed {
			return nil, siwcBadRequest(errors.New("SIWC code already submitted; start a new authorization"))
		}
		// Retain the issued registration before consuming a one-time code. A
		// fresh attempt can then reuse it after an exchange failure or restart.
		if err := siwc.SaveRegistration(s.siwcStateDir, id, siwc.Registration{ClientID: clientID,
			HostID: flow.protocol.HostID, AccountID: accountID, ProxyID: flow.proxyID, CreatedAt: time.Now()}); err != nil {
			return nil, siwcBadRequest(errors.New("cannot persist issued SIWC registration"))
		}
		flow.claimed = true
		flow.credential, err = client.Exchange(ctx, flow.protocol, code, clientID)
		if err != nil {
			return nil, siwcBadRequest(err)
		}
	}
	if len(flow.models) == 0 {
		flow.models, err = client.Models(ctx, flow.credential.AccessToken)
		if err != nil {
			return nil, siwcBadRequest(err)
		}
		if len(flow.models) == 0 {
			return nil, siwcBadRequest(errors.New("SIWC authorization has no listed models"))
		}
	}
	credentials := openAISiwcCredentials(*flow.credential)
	mapping := make(map[string]any, len(flow.models))
	for _, model := range flow.models {
		mapping[model] = model
	}
	credentials["model_mapping"] = mapping
	credentials["siwc_models"] = flow.models
	flow.account, err = create(credentials, flow.proxyID)
	return flow.account, err
}

func (s *OpenAIOAuthService) refreshSIWC(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	proxyURL, err := s.siwcProxy(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	client, err := newSIWCClient(proxyURL)
	if err != nil {
		return nil, err
	}
	credential, err := client.Refresh(ctx, siwcCredentialFromAccount(account))
	if err != nil {
		return nil, err
	}
	return &OpenAITokenInfo{SIWC: credential, AuthMode: siwc.AuthMode, AccessToken: credential.AccessToken,
		RefreshToken: credential.RefreshToken, IDToken: credential.IDToken, ClientID: credential.ClientID,
		Email: credential.Email, ExpiresAt: credential.ExpiresAt, ExpiresIn: credential.ExpiresAt - time.Now().Unix()}, nil
}

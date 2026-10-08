// Package siwc implements the separate ChatGPT token-sharing authorization flow.
package siwc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	AuthMode     = "siwc"
	Issuer       = "https://auth.openai.com"
	AuthorizeURL = Issuer + "/api/accounts/authorize"
	TokenURL     = Issuer + "/api/accounts/oauth/token"
	JWKSURL      = Issuer + "/.well-known/jwks.json"
	Resource     = "https://api.openai.com/v1"
	ResponsesURL = Resource + "/responses"
	Scopes       = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	UserAgent    = "Sub2API-SIWC/1.0"
	RedirectURI  = "http://127.0.0.1:1455/auth/callback"
	SessionTTL   = 10 * time.Minute
)

var clientIDPattern = regexp.MustCompile(`^oaiapp_[A-Za-z0-9_-]{1,256}$`)

type Session struct {
	State       string
	Nonce       string
	Verifier    string
	HostID      string
	ClientID    string
	Subject     string
	IDTokenHint string
	CreatedAt   time.Time
}

type Credential struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token,omitempty"`
	IDToken           string `json:"id_token"`
	ClientID          string `json:"client_id"`
	HostID            string `json:"ext_agent_host_id"`
	Subject           string `json:"subject"`
	Email             string `json:"email,omitempty"`
	Scope             string `json:"granted_scope"`
	ExpiresAt         int64  `json:"expires_at"`
	EarliestRefreshAt int64  `json:"earliest_refresh_at,omitempty"`
}

func HasSharingScopes(scope string) bool {
	values := strings.Fields(scope)
	invoke, direct := false, false
	for _, v := range values {
		invoke = invoke || v == "resource.invoke"
		direct = direct || v == "chatgpt.tokens.use.direct"
	}
	return invoke && direct
}

func NewSession(hostID string) (*Session, error) {
	if hostID == "" {
		hostID = uuid.New().URN()
	}
	if !strings.HasPrefix(hostID, "urn:uuid:") {
		return nil, errors.New("SIWC host ID must be a UUID URN")
	}
	if _, err := uuid.Parse(hostID); err != nil {
		return nil, errors.New("invalid SIWC host ID")
	}
	state, err := openai.GenerateState()
	if err != nil {
		return nil, err
	}
	nonce, err := openai.GenerateState()
	if err != nil {
		return nil, err
	}
	verifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	return &Session{State: state, Nonce: nonce, Verifier: verifier, HostID: hostID,
		ClientID: "dynamic_agent_client", CreatedAt: time.Now()}, nil
}

func (s *Session) AuthorizationURL() string {
	p := url.Values{"client_id": {s.ClientID}, "response_type": {"code"},
		"redirect_uri": {RedirectURI}, "scope": {Scopes}, "resource": {Resource},
		"state": {s.State}, "nonce": {s.Nonce}, "code_challenge_method": {"S256"},
		"code_challenge": {openai.GenerateCodeChallenge(s.Verifier)}, "ext_agent_host_id": {s.HostID}}
	if s.ClientID == "dynamic_agent_client" {
		p.Set("agent_name_hint", "Sub2API")
	} else if s.IDTokenHint != "" {
		p.Set("id_token_hint", s.IDTokenHint)
	}
	return AuthorizeURL + "?" + p.Encode()
}

// Callback validates the complete pasted URL before the session is consumed.
func (s *Session) Callback(raw string) (string, string, error) {
	if len(raw) > 16384 || time.Since(s.CreatedAt) > SessionTTL {
		return "", "", errors.New("SIWC callback invalid or expired")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Host != "127.0.0.1:1455" || u.Path != "/auth/callback" || u.User != nil || u.Fragment != "" {
		return "", "", errors.New("invalid SIWC callback URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) > 16 {
		return "", "", errors.New("invalid SIWC callback query")
	}
	for _, values := range q {
		if len(values) != 1 {
			return "", "", errors.New("duplicate SIWC callback parameter")
		}
	}
	if q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(s.State)) != 1 {
		return "", "", errors.New("SIWC state mismatch")
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		return "", "", errors.New("SIWC authorization was not granted")
	}
	clientID := q.Get("client_id")
	if s.ClientID != "dynamic_agent_client" {
		if clientID != "" && clientID != s.ClientID {
			return "", "", errors.New("SIWC client ID mismatch")
		}
		clientID = s.ClientID
	}
	if !clientIDPattern.MatchString(clientID) {
		return "", "", errors.New("missing or invalid issued SIWC client ID")
	}
	return q.Get("code"), clientID, nil
}

type Client struct{ http *http.Client }

// NewClient copies the supplied client so redirect/timeout changes never affect
// a shared pool. This client is used only for credential acquisition/validation.
func NewClient(client *http.Client) *Client {
	copyClient := *client
	copyClient.Timeout = 30 * time.Second
	copyClient.Jar = nil
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copyClient}
}

func (c *Client) json(ctx context.Context, endpoint string, form url.Values, bearer string, out any) error {
	if endpoint != TokenURL && endpoint != JWKSURL && endpoint != Resource+"/models" {
		return errors.New("invalid SIWC endpoint")
	}
	method := http.MethodGet
	var body io.Reader
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("cannot construct SIWC request")
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("SIWC authorization network request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return errors.New("invalid SIWC authorization response size")
	}
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if endpoint == TokenURL && json.Unmarshal(data, &failure) == nil {
			switch failure.Error {
			case "invalid_grant", "invalid_client", "invalid_scope", "unauthorized_client":
				return fmt.Errorf("SIWC authorization failed: %s", failure.Error)
			}
		}
		return fmt.Errorf("SIWC authorization endpoint returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("invalid SIWC authorization JSON")
	}
	return nil
}

type tokenResponse struct {
	AccessToken       string          `json:"access_token"`
	RefreshToken      string          `json:"refresh_token"`
	IDToken           string          `json:"id_token"`
	TokenType         string          `json:"token_type"`
	Scope             string          `json:"scope"`
	ExpiresIn         int64           `json:"expires_in"`
	EarliestRefreshAt json.RawMessage `json:"earliest_refresh_at"`
}

func (c *Client) Exchange(ctx context.Context, session *Session, code, clientID string) (*Credential, error) {
	keys, err := c.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	var result tokenResponse
	err = c.json(ctx, TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "code_verifier": {session.Verifier}, "redirect_uri": {RedirectURI}, "resource": {Resource}}, "", &result)
	if err != nil {
		return nil, err
	}
	return c.credential(result, &Credential{ClientID: clientID, HostID: session.HostID, Subject: session.Subject}, session.Nonce, true, keys)
}

func (c *Client) Refresh(ctx context.Context, previous Credential) (*Credential, error) {
	if !clientIDPattern.MatchString(previous.ClientID) || previous.RefreshToken == "" || previous.Subject == "" || !HasSharingScopes(previous.Scope) {
		return nil, errors.New("incomplete SIWC refresh credentials")
	}
	if previous.EarliestRefreshAt > time.Now().Unix() {
		return nil, errors.New("SIWC refresh is not available yet")
	}
	// Fetch verification keys before consuming a rotating refresh token. A
	// transient JWKS outage must not discard a successfully rotated grant.
	keys, err := c.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	var result tokenResponse
	if err := c.json(ctx, TokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {previous.RefreshToken},
		"client_id": {previous.ClientID}, "resource": {Resource}}, "", &result); err != nil {
		return nil, err
	}
	return c.credential(result, &previous, "", false, keys)
}

func (c *Client) credential(response tokenResponse, previous *Credential, nonce string, initial bool, keys []jwk) (*Credential, error) {
	if strings.TrimSpace(response.AccessToken) == "" || !strings.EqualFold(response.TokenType, "Bearer") || response.ExpiresIn <= 0 || response.ExpiresIn > 31536000 {
		return nil, errors.New("invalid SIWC token response")
	}
	result := *previous
	result.AccessToken, result.ExpiresAt = response.AccessToken, time.Now().Unix()+response.ExpiresIn
	result.EarliestRefreshAt = 0
	if len(response.EarliestRefreshAt) > 0 && string(response.EarliestRefreshAt) != "null" {
		var seconds int64
		if json.Unmarshal(response.EarliestRefreshAt, &seconds) == nil {
			result.EarliestRefreshAt = seconds
		} else {
			var stamp string
			if json.Unmarshal(response.EarliestRefreshAt, &stamp) != nil {
				return nil, errors.New("invalid SIWC refresh time")
			}
			t, err := time.Parse(time.RFC3339, stamp)
			if err != nil {
				return nil, errors.New("invalid SIWC refresh time")
			}
			result.EarliestRefreshAt = t.Unix()
		}
	}
	if response.Scope != "" || initial {
		result.Scope = response.Scope
	}
	if !HasSharingScopes(result.Scope) {
		return nil, errors.New("SIWC token-sharing scopes were not granted")
	}
	if response.RefreshToken != "" {
		result.RefreshToken = response.RefreshToken
	}
	if response.IDToken == "" {
		if initial {
			return nil, errors.New("SIWC ID token missing")
		}
		return &result, nil
	}
	claims, err := c.verifyIdentity(response.IDToken, result.ClientID, keys)
	if err != nil {
		return nil, err
	}
	subject, _ := claims["sub"].(string)
	if subject == "" || (result.Subject != "" && result.Subject != subject) {
		return nil, errors.New("SIWC subject mismatch")
	}
	if nonce != "" {
		actual, _ := claims["nonce"].(string)
		if subtle.ConstantTimeCompare([]byte(actual), []byte(nonce)) != 1 {
			return nil, errors.New("SIWC nonce mismatch")
		}
	}
	result.IDToken, result.Subject = response.IDToken, subject
	if email, ok := claims["email"].(string); ok {
		result.Email = email
	}
	return &result, nil
}

type jwk struct {
	KeyType    string   `json:"kty"`
	KeyID      string   `json:"kid"`
	Algorithm  string   `json:"alg"`
	Use        string   `json:"use"`
	Operations []string `json:"key_ops"`
	N          string   `json:"n"`
	E          string   `json:"e"`
	Curve      string   `json:"crv"`
	X          string   `json:"x"`
	Y          string   `json:"y"`
}

func (c *Client) signingKeys(ctx context.Context) ([]jwk, error) {
	var keys struct {
		Keys []jwk `json:"keys"`
	}
	if err := c.json(ctx, JWKSURL, nil, "", &keys); err != nil {
		return nil, err
	}
	if len(keys.Keys) == 0 {
		return nil, errors.New("SIWC signing keys unavailable")
	}
	return keys.Keys, nil
}

func (c *Client) verifyIdentity(raw, clientID string, keys []jwk) (jwt.MapClaims, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		var selected []jwk
		for _, key := range keys {
			if kid == "" || key.KeyID != kid || (key.Algorithm != "" && key.Algorithm != token.Method.Alg()) || (key.Use != "" && key.Use != "sig") {
				continue
			}
			if key.Operations != nil {
				verify := false
				for _, op := range key.Operations {
					verify = verify || op == "verify"
				}
				if !verify {
					continue
				}
			}
			selected = append(selected, key)
		}
		if len(selected) != 1 {
			return nil, errors.New("invalid signing key")
		}
		return selected[0].publicKey(token.Method.Alg())
	}, jwt.WithValidMethods([]string{"RS256", "ES256"}), jwt.WithIssuer(Issuer), jwt.WithAudience(clientID), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || token == nil || !token.Valid {
		return nil, errors.New("SIWC ID token verification failed")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["iat"] == nil {
		return nil, errors.New("SIWC ID token issued-at missing")
	}
	audience, err := claims.GetAudience()
	if err != nil {
		return nil, errors.New("invalid SIWC audience")
	}
	azp, hasAZP := claims["azp"]
	if (hasAZP && azp != clientID) || (len(audience) > 1 && azp != clientID) {
		return nil, errors.New("SIWC authorized-party mismatch")
	}
	return claims, nil
}

func (k jwk) publicKey(algorithm string) (any, error) {
	decode := base64.RawURLEncoding.DecodeString
	if k.KeyType == "RSA" && algorithm == "RS256" {
		n, en := decode(k.N)
		e, ee := decode(k.E)
		if en != nil || ee != nil || len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("invalid RSA key")
		}
		exp := new(big.Int).SetBytes(e).Int64()
		if exp < 3 || exp > 2147483647 || exp%2 == 0 {
			return nil, errors.New("invalid RSA exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exp)}, nil
	}
	if k.KeyType == "EC" && k.Curve == "P-256" && algorithm == "ES256" {
		x, ex := decode(k.X)
		y, ey := decode(k.Y)
		if ex != nil || ey != nil || len(x) != 32 || len(y) != 32 {
			return nil, errors.New("invalid EC key")
		}
		encoded := append(append([]byte{4}, x...), y...)
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), encoded)
		if err != nil {
			return nil, errors.New("invalid EC point")
		}
		return key, nil
	}
	return nil, errors.New("unsupported SIWC signing key")
}

func (c *Client) Models(ctx context.Context, accessToken string) ([]string, error) {
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err := c.json(ctx, Resource+"/models", nil, accessToken, &catalog); err != nil {
		return nil, err
	}
	if catalog.Models == nil {
		return nil, errors.New("SIWC model catalog is missing")
	}
	models := make([]string, 0, len(catalog.Models))
	seen := map[string]bool{}
	for _, model := range catalog.Models {
		if model.Visibility == "list" && model.Slug != "" && !seen[model.Slug] {
			models = append(models, model.Slug)
			seen[model.Slug] = true
		}
	}
	return models, nil
}

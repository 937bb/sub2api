package siwc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureResponse(value any) *http.Response {
	body, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func signedFixture(t *testing.T, session *Session, mutate func(jwt.MapClaims), responseChanges map[string]any) (*Client, *[]url.Values) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	claims := jwt.MapClaims{"iss": Issuer, "aud": "oaiapp_fixture", "sub": "subject-one", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": session.Nonce}
	if mutate != nil {
		mutate(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "fixture-key"
	raw, err := token.SignedString(key)
	require.NoError(t, err)
	publicKey, err := key.PublicKey.Bytes()
	require.NoError(t, err)
	var calls []url.Values
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, UserAgent, req.UserAgent())
		require.Empty(t, req.Header.Get("Cookie"))
		switch req.URL.String() {
		case TokenURL:
			require.NoError(t, req.ParseForm())
			calls = append(calls, req.Form)
			value := map[string]any{"access_token": "new-access", "refresh_token": "rotated-refresh", "token_type": "Bearer", "expires_in": 3600, "id_token": raw, "scope": Scopes}
			for k, v := range responseChanges {
				value[k] = v
			}
			return fixtureResponse(value), nil
		case JWKSURL:
			return fixtureResponse(map[string]any{"keys": []jwk{{KeyType: "EC", KeyID: "fixture-key", Curve: "P-256", Algorithm: "ES256", X: base64.RawURLEncoding.EncodeToString(publicKey[1:33]), Y: base64.RawURLEncoding.EncodeToString(publicKey[33:])}}}), nil
		default:
			t.Fatalf("unexpected endpoint %s", req.URL)
			return nil, nil
		}
	})})
	return client, &calls
}

func TestSIWCAuthorizationAndCallback(t *testing.T) {
	session, err := NewSession("")
	require.NoError(t, err)
	u, err := url.Parse(session.AuthorizationURL())
	require.NoError(t, err)
	require.Equal(t, AuthorizeURL, strings.Split(u.String(), "?")[0])
	require.Equal(t, session.HostID, u.Query().Get("ext_agent_host_id"))
	require.Equal(t, "S256", u.Query().Get("code_challenge_method"))
	callback := RedirectURI + "?" + url.Values{"code": {"code"}, "state": {session.State}, "client_id": {"oaiapp_fixture"}}.Encode()
	code, clientID, err := session.Callback(callback)
	require.NoError(t, err)
	require.Equal(t, "code", code)
	require.Equal(t, "oaiapp_fixture", clientID)
	for _, bad := range []string{callback + "&code=other", strings.Replace(callback, "127.0.0.1", "example.com", 1), strings.Replace(callback, session.State, "invalid", 1), callback + "#fragment"} {
		_, _, err := session.Callback(bad)
		require.Error(t, err)
	}
	session.CreatedAt = time.Now().Add(-SessionTTL - time.Second)
	_, _, err = session.Callback(callback)
	require.Error(t, err)
}

func TestSIWCExchangeAndRefreshIdentity(t *testing.T) {
	session, err := NewSession("")
	require.NoError(t, err)
	client, calls := signedFixture(t, session, nil, nil)
	credential, err := client.Exchange(context.Background(), session, "code", "oaiapp_fixture")
	require.NoError(t, err)
	require.Equal(t, session.HostID, credential.HostID)
	require.Equal(t, "subject-one", credential.Subject)
	require.Equal(t, session.Verifier, (*calls)[0].Get("code_verifier"))
	refreshed, err := client.Refresh(context.Background(), *credential)
	require.NoError(t, err)
	require.Equal(t, credential.HostID, refreshed.HostID)
	require.Equal(t, credential.ClientID, (*calls)[1].Get("client_id"))
	require.Equal(t, "refresh_token", (*calls)[1].Get("grant_type"))
	require.Equal(t, Resource, (*calls)[1].Get("resource"))
	credential.Subject = "different-account"
	_, err = client.Refresh(context.Background(), *credential)
	require.ErrorContains(t, err, "subject mismatch")
}

func TestSIWCRejectsInvalidGrants(t *testing.T) {
	for name, mutate := range map[string]func(jwt.MapClaims){
		"nonce":           func(c jwt.MapClaims) { c["nonce"] = "wrong" },
		"audience":        func(c jwt.MapClaims) { c["aud"] = "other-client" },
		"issuer":          func(c jwt.MapClaims) { c["iss"] = "https://invalid.example" },
		"expired":         func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"missing_iat":     func(c jwt.MapClaims) { delete(c, "iat") },
		"missing_subject": func(c jwt.MapClaims) { delete(c, "sub") },
		"azp":             func(c jwt.MapClaims) { c["azp"] = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			session, err := NewSession("")
			require.NoError(t, err)
			client, _ := signedFixture(t, session, mutate, nil)
			_, err = client.Exchange(context.Background(), session, "code", "oaiapp_fixture")
			require.Error(t, err)
		})
	}
	session, err := NewSession("")
	require.NoError(t, err)
	for _, changes := range []map[string]any{{"scope": "openid profile"}, {"token_type": "other"}, {"expires_in": 0}, {"access_token": ""}, {"id_token": ""}} {
		client, _ := signedFixture(t, session, nil, changes)
		_, err := client.Exchange(context.Background(), session, "code", "oaiapp_fixture")
		require.Error(t, err)
	}
}

func TestSIWCRefreshPreservesOmittedValues(t *testing.T) {
	session, err := NewSession("")
	require.NoError(t, err)
	client, calls := signedFixture(t, session, nil, map[string]any{"scope": "", "id_token": "", "refresh_token": ""})
	previous := Credential{ClientID: "oaiapp_fixture", Subject: "subject-one", Scope: Scopes, RefreshToken: "existing-refresh", IDToken: "old-id", HostID: session.HostID}
	result, err := client.Refresh(context.Background(), previous)
	require.NoError(t, err)
	require.Equal(t, previous.RefreshToken, result.RefreshToken)
	require.Equal(t, previous.Scope, result.Scope)
	previous.EarliestRefreshAt = time.Now().Add(time.Hour).Unix()
	_, err = client.Refresh(context.Background(), previous)
	require.Error(t, err)
	require.Len(t, *calls, 1)
}

func TestSIWCCatalogAndRedirectIsolation(t *testing.T) {
	count := 0
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		count++
		require.Equal(t, Resource+"/models", req.URL.String())
		require.Equal(t, "Bearer catalog-token", req.Header.Get("Authorization"))
		if count == 1 {
			return fixtureResponse(map[string]any{"models": []any{map[string]any{"slug": "model-one", "visibility": "list"}, map[string]any{"slug": "hidden", "visibility": "hidden"}}}), nil
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://invalid.example/steal"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	models, err := client.Models(context.Background(), "catalog-token")
	require.NoError(t, err)
	require.Equal(t, []string{"model-one"}, models)
	_, err = client.Models(context.Background(), "catalog-token")
	require.Error(t, err)
	require.Equal(t, 2, count)
}

func TestSIWCEmptyCatalogAndUnavailableSigningKeys(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == Resource+"/models" {
			return fixtureResponse(map[string]any{"models": []any{}}), nil
		}
		require.Equal(t, JWKSURL, req.URL.String(), "must not consume RT when key retrieval fails")
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})})
	models, err := client.Models(context.Background(), "token")
	require.NoError(t, err)
	require.Empty(t, models)
	_, err = client.Refresh(context.Background(), Credential{ClientID: "oaiapp_test", Subject: "user", RefreshToken: "rt", Scope: Scopes})
	require.ErrorContains(t, err, "503")
}

package siwc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestRecoverLegacySIWCVerifiesGrantBeforeRestoringMetadata(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pub, err := key.PublicKey.Bytes()
	require.NoError(t, err)
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, JWKSURL, req.URL.String(), "recovery must not rotate a refresh token")
		return fixtureResponse(map[string]any{"keys": []jwk{{KeyType: "EC", KeyID: "key", Curve: "P-256", X: base64.RawURLEncoding.EncodeToString(pub[1:33]), Y: base64.RawURLEncoding.EncodeToString(pub[33:])}}}), nil
	})})
	for _, name := range []string{"valid", "expired", "issuer", "audience", "client", "scope", "subject", "future", "unsigned"} {
		t.Run(name, func(t *testing.T) {
			claims := jwt.MapClaims{"iss": Issuer, "aud": Resource, "sub": "user", "client_id": "oaiapp_test", "scope": Scopes, "iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": time.Now().Add(time.Hour).Unix()}
			switch name {
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "issuer":
				claims["iss"] = "https://wrong.example"
			case "audience":
				claims["aud"] = "codex"
			case "client":
				claims["client_id"] = "other"
			case "scope":
				claims["scope"] = "openid"
			case "subject":
				claims["sub"] = ""
			case "future":
				claims["iat"] = time.Now().Add(time.Minute).Unix()
			}
			token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
			token.Header["kid"] = "key"
			raw, err := token.SignedString(key)
			require.NoError(t, err)
			if name == "unsigned" {
				raw = raw[:len(raw)-20]
			}
			result, err := client.RecoverLegacy(context.Background(), Credential{AccessToken: raw, ClientID: "oaiapp_test", RefreshToken: "rt"})
			if name != "valid" && name != "expired" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "user", result.Subject)
			require.Equal(t, claims["exp"], result.ExpiresAt)
			require.Equal(t, "rt", result.RefreshToken)
			require.Empty(t, result.HostID, "do not invent the original device identity")
		})
	}
}

package siwc

import (
	"context"
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// RecoverLegacy restores metadata from a signed access token, not from an
// administrator's display label. An expired token can identify a refresh grant
// but remains expired; this method never exchanges or rotates refresh tokens.
func (c *Client) RecoverLegacy(ctx context.Context, previous Credential) (*Credential, error) {
	if !clientIDPattern.MatchString(previous.ClientID) || previous.AccessToken == "" {
		return nil, errors.New("SIWC access token and issued client ID are required")
	}
	keys, err := c.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := verifySignedToken(previous.AccessToken, keys, jwt.WithoutClaimsValidation())
	if err != nil {
		return nil, err
	}
	expires, err := claims.GetExpirationTime()
	if err != nil || expires == nil {
		return nil, errors.New("SIWC access token expiry missing")
	}
	// Validate issuer, audience, nbf and iat even for historical grants. Removing
	// exp is confined to metadata recovery, never request authorization.
	validationClaims := jwt.MapClaims{}
	for key, value := range claims {
		if key != "exp" {
			validationClaims[key] = value
		}
	}
	validator := jwt.NewValidator(jwt.WithIssuer(Issuer), jwt.WithAudience(Resource), jwt.WithIssuedAt())
	if validator.Validate(validationClaims) != nil {
		return nil, errors.New("SIWC access token claims invalid")
	}
	issued, err := claims.GetIssuedAt()
	if err != nil || issued == nil || !expires.After(issued.Time) {
		return nil, errors.New("SIWC access token lifetime invalid")
	}
	subject, _ := claims["sub"].(string)
	scope, _ := claims["scope"].(string)
	if claims["client_id"] != previous.ClientID || subject == "" || (previous.Subject != "" && subject != previous.Subject) || !HasSharingScopes(scope) {
		return nil, errors.New("SIWC access token identity or sharing scopes mismatch")
	}
	previous.Subject, previous.Scope, previous.ExpiresAt = subject, scope, expires.Unix()
	return &previous, nil
}

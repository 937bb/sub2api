package openai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

type transportCredentialKey struct{}

// WithTransportCredential isolates auth requests before an account/device exists.
// Only a one-way digest is kept in context; nothing is added to the wire request.
func WithTransportCredential(ctx context.Context, kind, credential string) context.Context {
	sum := sha256.Sum256([]byte(kind + "\x00" + credential))
	return context.WithValue(ctx, transportCredentialKey{}, hex.EncodeToString(sum[:]))
}

// TransportIdentityKey describes connection identity, excluding per-turn fields.
// A refreshed bearer does not rotate a known device. Requests lacking a device
// fall back to a credential digest rather than sharing another user's connection.
func TransportIdentityKey(ctx context.Context, h http.Header) string {
	fields := []string{
		identityHeader(h, "user-agent"), identityHeader(h, "originator"),
		identityHeader(h, "version"), identityHeader(h, "chatgpt-account-id"),
		identityHeader(h, "x-codex-installation-id"), identityHeader(h, "x-openai-fedramp"),
	}
	if fields[4] == "" {
		credential, _ := ctx.Value(transportCredentialKey{}).(string)
		if credential == "" {
			sum := sha256.Sum256([]byte(identityHeader(h, "authorization")))
			credential = hex.EncodeToString(sum[:])
		}
		fields = append(fields, credential)
	}
	encoded, _ := json.Marshal(fields)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// Canonical spelling wins; otherwise choose a deterministic case alias.
func identityHeader(h http.Header, name string) string {
	if value := h.Get(name); value != "" {
		return strings.TrimSpace(value)
	}
	key, value := "", ""
	for candidate, values := range h {
		if strings.EqualFold(candidate, name) && len(values) > 0 && (key == "" || candidate < key) {
			key, value = candidate, values[0]
		}
	}
	return strings.TrimSpace(value)
}

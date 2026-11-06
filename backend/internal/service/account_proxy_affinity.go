package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type accountProxyAffinityKey struct{}

// Capture the original client identity before protocol conversion or fingerprint
// convergence. An explicitly empty scope also survives nested forwarders: a
// generated account-wide session ID must never become a conversation identity.
func withAccountProxyAffinity(ctx context.Context, c *gin.Context, body []byte) context.Context {
	if _, captured := ctx.Value(accountProxyAffinityKey{}).(string); captured {
		return ctx
	}
	scope, _ := resolveOpenAIWSExecutionScope(c, body, getAPIKeyIDFromContext(c))
	if scope == "" {
		// Messages clients carry a real conversation ID in the Claude Code header
		// or structured user_id. Arbitrary user IDs and content hashes are not
		// conversation identities and must not pin all of a user's traffic.
		sessionID := ClaudeCodeSessionIDFromHeader(c)
		if sessionID == "" {
			if metadata := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String()); metadata != nil {
				sessionID = strings.TrimSpace(metadata.SessionID)
			}
		}
		if sessionID != "" {
			scope, _ = deriveOpenAISessionHashes(openAIWSExecutionScopeSeed(getAPIKeyIDFromContext(c), "session", sessionID, ""))
		}
	}
	return context.WithValue(ctx, accountProxyAffinityKey{}, scope)
}

// Rendezvous hashing gives HTTP and WS workers the same route without mutable
// process-local bindings. Hash stable proxy IDs, never credentials or input text.
// A scoped conversation has exactly one route; even a pre-handshake failure must
// not move it to another IP. Anonymous traffic keeps legacy connection fallback.
func accountProxyRoutesForContext(ctx context.Context, account *Account, legacyURL string) ([]string, error) {
	scope, _ := ctx.Value(accountProxyAffinityKey{}).(string)
	if scope == "" || account == nil || account.Platform != PlatformOpenAI || len(AccountProxyFallbackIDs(account.Extra)) == 0 {
		return rotatingAccountProxyRoutes(account, legacyURL)
	}
	if _, err := accountProxyRoutes(account, legacyURL); err != nil {
		return nil, err
	}
	ids := AccountProxyFallbackIDs(account.Extra)
	if account.ProxyID != nil {
		ids = append(ids, *account.ProxyID)
	}
	var selectedID int64
	var best [sha256.Size]byte
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		score := sha256.Sum256([]byte(fmt.Sprintf("account-proxy-v1:%d:%d:%s", account.ID, id, scope)))
		comparison := bytes.Compare(score[:], best[:])
		if selectedID == 0 || comparison > 0 || (comparison == 0 && id < selectedID) {
			selectedID, best = id, score
		}
	}
	// Hash all configured IDs, including temporarily unavailable proxies. A stale
	// snapshot or disabled route must fail closed rather than silently remap a
	// conversation. Explicitly changing membership is the remapping boundary.
	if account.ProxyID != nil && *account.ProxyID == selectedID {
		if p := account.Proxy; p != nil && p.ID == selectedID && usableAccountProxy(p) {
			return []string{p.URL()}, nil
		}
	} else {
		for _, p := range account.FallbackProxies {
			if p != nil && p.ID == selectedID && usableAccountProxy(p) {
				return []string{p.URL()}, nil
			}
		}
	}
	return nil, fmt.Errorf("bound conversation proxy unavailable (proxy_id=%d)", selectedID)
}

package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// Each operation retains the account's identity and dedicated proxy policy.
// IDE reference calls close their HTTP connections; do not reuse a connection
// after an account environment edit or follow redirects carrying credentials.
func prepareKiroHTTPRequest(req *http.Request, credentials map[string]any, token string, operation kiro.Operation) *http.Request {
	kiro.OperationHeaders(req.Header, credentials, token, operation)
	req.Close = true
	ctx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileKiro)
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	return req.WithContext(ctx)
}

func preserveKiroAccountEnvironment(account *Account, incoming map[string]any) map[string]any {
	if account == nil || account.Platform != PlatformKiro {
		return incoming
	}
	return kiro.PreserveEnvironment(account.Credentials, incoming)
}

// Merge aliases before retaining redacted secrets; otherwise a newly imported
// refreshToken conflicts with the old refresh_token during normalization.
func mergeAccountCredentials(account *Account, incoming map[string]any) map[string]any {
	if account.Platform != PlatformKiro {
		return MergePreservingSensitiveCreds(account.Credentials, incoming)
	}
	incoming = preserveKiroAccountEnvironment(account, incoming)
	existing := make(map[string]any, len(account.Credentials))
	for k, v := range account.Credentials {
		existing[k] = v
	}
	nested, _ := incoming["credentials"].(map[string]any)
	aliases := map[string]string{"access_token": "accessToken", "refresh_token": "refreshToken", "id_token": "idToken", "client_secret": "clientSecret", "api_key": "kiroApiKey"}
	for canonical, alias := range aliases {
		_, a := incoming[canonical]
		_, b := incoming[alias]
		_, c := nested[canonical]
		_, d := nested[alias]
		if a || b || c || d {
			delete(existing, canonical)
			delete(existing, alias)
		}
	}
	return MergePreservingSensitiveCreds(existing, incoming)
}

package service

import (
	"net/http"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/net/http/httpguts"
)

// AccountExtraUpstreamRequestIDHeader selects an explicit response header.
// Unconfigured accounts automatically recognize supported upstream headers.
const AccountExtraUpstreamRequestIDHeader = "upstream_request_id_header"

const (
	maxUpstreamRequestIDHeaderNameLen = 64
	// Keep IDs within the usage_logs.upstream_request_id column width.
	maxUsageUpstreamRequestIDLen = 128
)

// UpstreamRequestIDHeaderName returns an explicit account override, if any.
func UpstreamRequestIDHeaderName(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetExtraString(AccountExtraUpstreamRequestIDHeader))
}

// Recognize relay IDs before provider IDs, which a relay may also forward.
var upstreamRequestIDHeaders = []string{
	"X-Oneapi-Request-Id", "X-Client-Request-ID", "Request-Id", "X-Request-ID",
	"Xai-Request-Id", "X-Goog-Request-Id", "X-Amzn-Requestid",
}

// ResolveUpstreamRequestID returns the response header name and its ID.
// An explicit override is authoritative, including when that header is absent.
func ResolveUpstreamRequestID(account *Account, h http.Header) (string, string) {
	names := upstreamRequestIDHeaders
	if name := UpstreamRequestIDHeaderName(account); name != "" {
		names = []string{name}
	}
	for _, name := range names {
		id := strings.TrimSpace(h.Get(name))
		if id == "" || !httpguts.ValidHeaderFieldValue(id) {
			continue
		}
		return name, id
	}
	return "", ""
}

// UpstreamRequestIDFromHeaders reads an explicit or automatically detected ID.
func UpstreamRequestIDFromHeaders(account *Account, h http.Header) string {
	_, id := ResolveUpstreamRequestID(account, h)
	return id
}

// usageUpstreamRequestIDPtr keeps HTTP request IDs separate from WS response IDs.
// Overlong IDs are truncated without splitting a UTF-8 character.
func usageUpstreamRequestIDPtr(account *Account, h http.Header, wsMode bool) *string {
	if wsMode {
		return nil
	}
	id := UpstreamRequestIDFromHeaders(account, h)
	if id == "" {
		return nil
	}
	if len(id) > maxUsageUpstreamRequestIDLen {
		id = id[:maxUsageUpstreamRequestIDLen]
		for len(id) > 0 && !utf8.ValidString(id) {
			id = id[:len(id)-1]
		}
	}
	if id == "" {
		return nil
	}
	return &id
}

// ValidateUpstreamRequestIDHeaderExtra validates an explicit header override.
// Empty values remove the override and restore automatic detection.
func ValidateUpstreamRequestIDHeaderExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, ok := extra[AccountExtraUpstreamRequestIDHeader]
	if !ok || raw == nil {
		return nil
	}
	name, ok := raw.(string)
	if !ok {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a string")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		delete(extra, AccountExtraUpstreamRequestIDHeader)
		return nil
	}
	if len(name) > maxUpstreamRequestIDHeaderNameLen || !httpguts.ValidHeaderFieldName(name) {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a valid HTTP header name of at most 64 bytes")
	}
	extra[AccountExtraUpstreamRequestIDHeader] = name
	return nil
}

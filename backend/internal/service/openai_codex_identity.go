package service

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
)

// Minimum inference version retained for upstream compatibility.
const codexUpstreamMinVersion = "0.144.0"

// Bound administrator-supplied version strings before using them in headers.
const codexClientVersionMaxLen = 64

// Accept numeric releases and prerelease suffixes.
var codexClientVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$`)

// NormalizeCodexClientVersion rejects malformed header versions.
func NormalizeCodexClientVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > codexClientVersionMaxLen || !codexClientVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

// buildCodexCLIUserAgent uses the shared CLI profile.
func buildCodexCLIUserAgent(version string) string {
	if version = NormalizeCodexClientVersion(version); version == "" {
		return codexCLIUserAgent
	}
	return openai.SetCodexUserAgentVersion(codexCLIUserAgent, version)
}

// Enforcement defaults on; disabling it preserves a valid incoming client profile.
var codexIdentityEnforcement = func() *atomic.Bool {
	v := &atomic.Bool{}
	v.Store(true)
	return v
}()

// SetCodexIdentityEnforcementEnabled publishes the process-wide setting.
func SetCodexIdentityEnforcementEnabled(enabled bool) {
	codexIdentityEnforcement.Store(enabled)
}

// Runtime settings resolve the canonical profile without database work per request.
type codexCanonicalUserAgentResolver func() string

var (
	codexCanonicalUAMu       sync.RWMutex
	codexCanonicalUAResolver codexCanonicalUserAgentResolver
)

// SetCodexCanonicalUserAgentResolver installs the runtime profile resolver.
func SetCodexCanonicalUserAgentResolver(resolver func() string) {
	codexCanonicalUAMu.Lock()
	defer codexCanonicalUAMu.Unlock()
	codexCanonicalUAResolver = resolver
}

// CodexCanonicalUserAgent shares the inference identity resolver.
func CodexCanonicalUserAgent() string {
	return resolveCodexOutboundIdentity("").userAgent
}

// CodexCanonicalAuthIdentity returns the authentication identity without inference version.
func CodexCanonicalAuthIdentity() (userAgent, originator string) {
	identity := resolveCodexOutboundIdentity("")
	return identity.userAgent, identity.originator
}

// ApplyCodexCanonicalAuthIdentity replaces the authentication identity pair.
func ApplyCodexCanonicalAuthIdentity(h http.Header) {
	if h == nil {
		return
	}
	userAgent, originator := CodexCanonicalAuthIdentity()
	setCodexIdentityHeader(h, "user-agent", userAgent)
	setCodexIdentityHeader(h, "originator", originator)
	setCodexIdentityHeader(h, "version", "")
}

// CodexCanonicalClientVersion returns the effective engine version.
func CodexCanonicalClientVersion() string {
	return resolveCodexOutboundIdentity("").version
}

// codexCanonicalUserAgent falls back to the compiled profile.
func codexCanonicalUserAgent() string {
	codexCanonicalUAMu.RLock()
	resolver := codexCanonicalUAResolver
	codexCanonicalUAMu.RUnlock()
	if resolver != nil {
		if ua := resolver(); strings.TrimSpace(ua) != "" {
			return ua
		}
	}
	return codexCLIUserAgent
}

// All three fields describe the same client and engine version.
type codexOutboundIdentity struct {
	userAgent  string
	originator string
	version    string
}

// resolveCodexOutboundIdentity preserves an explicit client/platform override while
// deriving its engine version from the canonical runtime settings.
func resolveCodexOutboundIdentity(candidateUA string) codexOutboundIdentity {
	canonical := codexCanonicalUserAgent()
	if _, _, ok := openai.PairCodexClientIdentity(canonical); !ok || openai.CodexUserAgentVersion(canonical) == "" {
		canonical = codexCLIUserAgent
	}
	ua := candidateUA
	if strings.TrimSpace(ua) == "" {
		ua = canonical
	}
	originator, pairedUA, ok := openai.PairCodexClientIdentity(ua)
	if !ok {
		if originator, pairedUA, ok = openai.PairCodexClientIdentity(canonical); !ok {
			originator, pairedUA = openai.CodexCLIOriginator, codexCLIUserAgent
		}
	}
	// Preserve the production TUI-to-CLI compatibility normalization.
	originator, pairedUA, _ = openai.NormalizeCodexClientIdentityToCLI(originator, pairedUA)
	// Runtime settings own the engine version, including for account overrides.
	version := codexClientVersionFromUA(canonical)
	if rebuilt := openai.SetCodexUserAgentVersion(pairedUA, version); rebuilt != "" {
		pairedUA = rebuilt
	} else {
		originator, pairedUA, _ = openai.PairCodexClientIdentity(canonical)
		pairedUA = openai.SetCodexUserAgentVersion(pairedUA, version)
	}
	return codexOutboundIdentity{userAgent: pairedUA, originator: originator, version: version}
}

// codexClientVersionFromUA validates the engine version against the minimum.
func codexClientVersionFromUA(ua string) string {
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(ua))
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		return codexCLIVersion
	}
	return version
}

// ensureCodexIdentityHeaders fills identity fields without choosing transport features.
func ensureCodexIdentityHeaders(h http.Header) {
	if h == nil {
		return
	}
	identity := resolveCodexOutboundIdentity("")
	if strings.TrimSpace(codexIdentityHeaderValue(h, "user-agent")) == "" {
		h.Set("user-agent", identity.userAgent)
	}
	if strings.TrimSpace(codexIdentityHeaderValue(h, "originator")) == "" {
		h.Set("originator", identity.originator)
	}
	if strings.TrimSpace(codexIdentityHeaderValue(h, "version")) == "" {
		h.Set("version", identity.version)
	}
}

// applyOpenAICodexProbeHeaders constructs a consistent synthetic probe identity.
func applyOpenAICodexProbeHeaders(h http.Header) {
	if h == nil {
		return
	}
	ensureCodexIdentityHeaders(h)
	enforceCodexIdentityHeaders(h)
	h.Set("X-Codex-Window-ID", uuid.NewString())
}

// enforceCodexIdentityHeaders finalizes identity without an account override.
func enforceCodexIdentityHeaders(h http.Header) {
	enforceCodexIdentityHeadersWithUA(h, "")
}

// enforceCodexIdentityHeadersWithUA finalizes identity after all UA assignments.
// Missing originator deliberately opts compatibility bridges out. Transport-specific
// beta tokens remain owned by their builders; the obsolete HTTP beta is removed.
func enforceCodexIdentityHeadersWithUA(h http.Header, overrideUA string) {
	if h == nil || codexIdentityHeaderValue(h, "originator") == "" {
		return
	}
	stripOpenAILegacyResponsesBeta(h)
	if !codexIdentityEnforcement.Load() {
		pairCodexIdentityHeaders(h)
		return
	}
	identity := resolveCodexOutboundIdentity(overrideUA)
	setCodexIdentityHeader(h, "user-agent", identity.userAgent)
	setCodexIdentityHeader(h, "originator", identity.originator)
	setCodexIdentityHeader(h, "version", identity.version)
}

// pairCodexIdentityHeaders preserves the client profile but repairs the entire version pair.
func pairCodexIdentityHeaders(h http.Header) {
	originator, pairedUA, ok := openai.PairCodexClientIdentity(codexIdentityHeaderValue(h, "user-agent"))
	if !ok {
		identity := resolveCodexOutboundIdentity("")
		originator, pairedUA = identity.originator, identity.userAgent
	}
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(pairedUA))
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		version = resolveCodexOutboundIdentity("").version
	}
	pairedUA = openai.SetCodexUserAgentVersion(pairedUA, version)
	if pairedUA == "" {
		identity := resolveCodexOutboundIdentity("")
		originator, pairedUA, version = identity.originator, identity.userAgent, identity.version
	}
	setCodexIdentityHeader(h, "user-agent", pairedUA)
	setCodexIdentityHeader(h, "originator", originator)
	setCodexIdentityHeader(h, "version", version)
}

// Header maps created by plugins can contain noncanonical keys. Collapse aliases
// before writing so HTTP and WS never serialize conflicting identity values.
func setCodexIdentityHeader(h http.Header, name, value string) {
	for key := range h {
		if strings.EqualFold(key, name) {
			delete(h, key)
		}
	}
	if value != "" {
		h.Set(name, value)
	}
}

func codexIdentityHeaderValue(h http.Header, name string) string {
	if value := h.Get(name); value != "" {
		return value
	}
	matchedKey, value := "", ""
	for key, values := range h {
		if strings.EqualFold(key, name) && len(values) > 0 && (matchedKey == "" || key < matchedKey) {
			matchedKey, value = key, values[0]
		}
	}
	return value
}

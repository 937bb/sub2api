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

// codexUpstreamMinVersion 上游 /backend-api/codex 接受的最低 version 头：
// 若请求携带 version 且低于该值，上游直接 404（issue #3901，2026-07 实测）。
const codexUpstreamMinVersion = "0.144.0"

// codexClientVersionMaxLen 官方版本号均为短 ASCII 串，远低于此上限。
const codexClientVersionMaxLen = 64

// codexClientVersionPattern 允许 0.146.0 与 0.147.0-alpha.4 两类官方形态。
var codexClientVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$`)

// NormalizeCodexClientVersion 校验并归一化 Codex 客户端版本号，非法值返回空串。
// 该值会被拼进出站 User-Agent 与 version 头，必须拒绝任意字节，避免管理员误填或
// 自动同步拿到异常值时把不可控内容透给上游。
func NormalizeCodexClientVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > codexClientVersionMaxLen || !codexClientVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

// buildCodexCLIUserAgent builds the official interactive Codex TUI User-Agent.
// app-server writes clientInfo into the final parenthesized suffix, so both
// version declarations must be rebuilt from the same release.
func buildCodexCLIUserAgent(version string) string {
	if version = NormalizeCodexClientVersion(version); version == "" {
		return codexCLIUserAgent
	}
	return openai.CodexDefaultOriginator + "/" + version + codexCLIUserAgentEnvironment + " (" + openai.CodexDefaultOriginator + "; " + version + ")"
}

// normalizeCodexResponsesTransportHeaders is the final HTTP wire boundary for
// ChatGPT Codex Responses requests. Builders may keep accepting historical
// downstream aliases, but none of those aliases should reach the upstream.
//
// Official codex-rs 0.154.0 sends session-id, thread-id and
// x-client-request-id. It does not send the historical session_id,
// conversation_id, version or responses=experimental headers on ordinary HTTP
// Responses requests.
func normalizeCodexResponsesTransportHeaders(req *http.Request, account *Account) {
	if req == nil || req.URL == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	if !strings.EqualFold(req.URL.Hostname(), "chatgpt.com") || !strings.HasPrefix(req.URL.Path, "/backend-api/codex/responses") {
		return
	}

	// ensureCodexIdentityHeaders still supports legacy synthetic probes and adds
	// responses=experimental. Preserve only the caller's beta negotiations so
	// that helper cannot reintroduce the obsolete token at the wire boundary.
	requestedBeta := append([]string(nil), req.Header.Values("OpenAI-Beta")...)
	ensureCodexIdentityHeaders(req.Header)
	req.Header.Del("OpenAI-Beta")
	for _, value := range requestedBeta {
		req.Header.Add("OpenAI-Beta", value)
	}
	enforceCodexIdentityHeadersWithUA(req.Header, codexAccountUserAgent(account))

	sessionID := firstNonEmptyCodexHeader(req.Header, "session-id", "session_id")
	threadID := firstNonEmptyCodexHeader(req.Header, "thread-id", "x-client-request-id")
	if threadID == "" {
		threadID = codexThreadIDFromWindowHeader(req.Header.Get("x-codex-window-id"))
	}
	if sessionID != "" {
		req.Header.Set("session-id", sessionID)
	}
	if threadID != "" {
		req.Header.Set("thread-id", threadID)
		req.Header.Set("x-client-request-id", threadID)
	}

	req.Header.Del("session_id")
	req.Header.Del("conversation_id")
	req.Header.Del("version")
	req.Header.Del("accept-language")
	stripOpenAILegacyResponsesBeta(req.Header)
}

// normalizeCodexWebSocketTransportHeaders applies the same official identity
// vocabulary used by codex-rs to a ChatGPT Responses websocket handshake.
// Builders may use underscore aliases internally for compatibility and
// isolation, but those aliases are not part of the current Codex wire shape.
func normalizeCodexWebSocketTransportHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}

	sessionID := firstNonEmptyCodexHeader(headers, "session-id", "session_id")
	threadID := firstNonEmptyCodexHeader(headers, "thread-id", "x-client-request-id", "conversation_id")
	if sessionID == "" {
		sessionID = threadID
	}
	if threadID == "" {
		threadID = sessionID
	}
	if sessionID != "" {
		headers.Set("session-id", sessionID)
	}
	if threadID != "" {
		headers.Set("thread-id", threadID)
		headers.Set("x-client-request-id", threadID)
	}

	headers.Del("session_id")
	headers.Del("conversation_id")
	headers.Del("version")
	headers.Del("accept-language")
}

func firstNonEmptyCodexHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func codexThreadIDFromWindowHeader(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	threadID, generation, ok := strings.Cut(windowID, ":")
	if !ok || generation == "" {
		return ""
	}
	if _, err := uuid.Parse(threadID); err != nil {
		return ""
	}
	return threadID
}

// codexAccountUserAgent returns the Codex client UA used by an OAuth-like
// account. Explicit account and panel settings keep their priority. Without an
// override, all accounts use the canonical UA built from Codex's official
// default originator and current engine version. Account isolation belongs in
// installation/session/thread identifiers, not in invented OS profiles.
func codexAccountUserAgent(account *Account) string {
	if !isCodexAccountIdentityCandidate(account) {
		return ""
	}
	if configured := strings.TrimSpace(account.GetOpenAIUserAgent()); configured != "" {
		return configured
	}
	if configured := codexConfiguredUserAgent(); configured != "" {
		return configured
	}

	return codexCanonicalUserAgent()
}

// isCodexAccountIdentityCandidate recognizes OpenAI OAuth-like accounts,
// including legacy rows whose platform field was left empty. The empty
// platform case is safe here because callers use this helper only for the
// ChatGPT/Codex protocol paths identified by the account type.
func isCodexAccountIdentityCandidate(account *Account) bool {
	if account == nil || (account.Type != AccountTypeOAuth && account.Type != AccountTypeSetupToken) {
		return false
	}
	return account.Platform == "" || account.IsOpenAIOAuthLike()
}

// codexIdentityEnforcement 控制 enforceCodexIdentityHeaders 是否强制统一出站身份，
// 由 gateway.disable_codex_identity_enforcement 在服务构造时取反发布。
// 默认开启：上游在容量紧张时按客户端身份分优先级降载，被降载的请求会拿到
// HTTP 200 + 流内 server_is_overloaded，本次请求即失败；强制统一出口可确保没有
// 请求带着第三方或陈旧身份出站。关闭后退回「仅按最终 UA 配对 originator」的收口语义。
var codexIdentityEnforcement = func() *atomic.Bool {
	v := &atomic.Bool{}
	v.Store(true)
	return v
}()

// SetCodexIdentityEnforcementEnabled 发布 Codex 出站身份强制统一开关。
// enforceCodexIdentityHeaders 是所有出站路径共用的纯函数收口点，无法在热路径注入配置，
// 故由持有配置的服务在构造时发布进程级快照。
func SetCodexIdentityEnforcementEnabled(enabled bool) {
	codexIdentityEnforcement.Store(enabled)
}

// codexCanonicalUserAgentResolver 返回当前生效的规范 Codex User-Agent（后台设置 / 自动同步版本号）。
// 由 SettingService 在装配时注入；解析器内部自带 TTL 缓存，热路径不触库。
type codexCanonicalUserAgentResolver func() string

var (
	codexCanonicalUAMu        sync.RWMutex
	codexCanonicalUAResolver  codexCanonicalUserAgentResolver
	codexConfiguredUAResolver func() string
)

// SetCodexConfiguredUserAgentResolver supplies the explicit panel override.
// An empty value must stay empty so the per-account fallback remains available.
func SetCodexConfiguredUserAgentResolver(resolver func() string) {
	codexCanonicalUAMu.Lock()
	defer codexCanonicalUAMu.Unlock()
	codexConfiguredUAResolver = resolver
}

func codexConfiguredUserAgent() string {
	codexCanonicalUAMu.RLock()
	resolver := codexConfiguredUAResolver
	codexCanonicalUAMu.RUnlock()
	if resolver != nil {
		return strings.TrimSpace(resolver())
	}
	return ""
}

// SetCodexCanonicalUserAgentResolver 注入规范 User-Agent 解析器。
// 未注入或解析结果非法时回退到编译期常量 codexCLIUserAgent。
func SetCodexCanonicalUserAgentResolver(resolver func() string) {
	codexCanonicalUAMu.Lock()
	defer codexCanonicalUAMu.Unlock()
	codexCanonicalUAResolver = resolver
}

// CodexCanonicalUserAgent 返回当前生效的规范 Codex User-Agent。
// 取值走与推理相同的解析链：面板 UA 指纹 + 面板/自动同步版本号 + 编译期兜底。
// 供无账号句柄的出站路径（OAuth 换 Token / 刷新）使用。
func CodexCanonicalUserAgent() string {
	return resolveCodexOutboundIdentity("").userAgent
}

// CodexCanonicalAuthIdentity 返回凭据面（auth.openai.com：换 Token / 刷新 / whoami）
// 出站请求的身份对：规范 User-Agent 与配套 originator，与推理解析链同源。
// 凭据面不发 version 头——真实 Codex 客户端在该面只携带 originator 与 User-Agent
// （codex-rs login/default_client.rs 的 default_headers()），version 门槛
// （issue #3901）只存在于 /backend-api/codex 推理面。
func CodexCanonicalAuthIdentity() (userAgent, originator string) {
	identity := resolveCodexOutboundIdentity("")
	return identity.userAgent, identity.originator
}

// ApplyCodexCanonicalAuthIdentity 为凭据面出站请求写入身份对（不含 version）。
func ApplyCodexCanonicalAuthIdentity(h http.Header) {
	if h == nil {
		return
	}
	userAgent, originator := CodexCanonicalAuthIdentity()
	h.Set("user-agent", userAgent)
	h.Set("originator", originator)
}

// CodexCanonicalClientVersion 返回当前生效的 Codex 客户端版本号。
func CodexCanonicalClientVersion() string {
	return resolveCodexOutboundIdentity("").version
}

// codexCanonicalUserAgent 返回出站规范 User-Agent。
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

// codexOutboundIdentity 出站身份三元组，三者必须同源自洽：
// originator 与 User-Agent 首段配套（否则上游 404，issue #3901），
// version 等于 User-Agent 的版本段且不低于上游门槛。
type codexOutboundIdentity struct {
	userAgent  string
	originator string
	version    string
}

// resolveCodexOutboundIdentity 由候选 User-Agent 推导自洽的出站身份。
// candidateUA 为空时使用规范 User-Agent；推导不出官方身份时整体回退为规范 TUI 身份。
//
// 候选 UA（面板 / 账号级的管理员显式配置）只贡献客户端名与 OS / 架构 / 终端指纹，
// 其自带的版本段一律用当前生效版本重建：一条填写于某个历史版本的 UA 否则会把出站身份
// 永久钉死在陈旧版本上，绕过版本自动同步，落回上游优先降载的那一侧。
// 需要固定版本请填「Codex 客户端版本号」并关闭自动同步。
func resolveCodexOutboundIdentity(candidateUA string) codexOutboundIdentity {
	canonical := codexCanonicalUserAgent()
	if _, _, ok := openai.PairCodexClientIdentity(canonical); !ok {
		canonical = codexCLIUserAgent
	}
	ua := candidateUA
	if strings.TrimSpace(ua) == "" {
		ua = canonical
	}
	originator, pairedUA, ok := openai.PairCodexClientIdentity(ua)
	if !ok {
		if originator, pairedUA, ok = openai.PairCodexClientIdentity(canonical); !ok {
			originator, pairedUA = openai.CodexDefaultOriginator, codexCLIUserAgent
		}
	}
	// 生效版本只有一个来源：规范身份（面板版本号 → 自动同步值 → 内置常量，见
	// SettingService.GetOpenAICodexClientVersion）。UA 与 version 头由此同源派生。
	version := codexClientVersionFromUA(canonical)
	if rebuilt := openai.SetCodexUserAgentVersion(pairedUA, version); rebuilt != "" {
		pairedUA = rebuilt
	}
	return codexOutboundIdentity{userAgent: pairedUA, originator: originator, version: version}
}

// codexClientVersionFromUA 取 UA 的版本段作为生效版本；
// 非法或低于上游门槛（低于则上游 404，issue #3901）时回退编译期常量。
func codexClientVersionFromUA(ua string) string {
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(ua))
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		return codexCLIVersion
	}
	return version
}

// ensureCodexIdentityHeaders 补齐 OAuth（ChatGPT 内部接口）出站请求所需的 Codex 身份头。
// 已有 User-Agent 与 version 保持不变，交给紧随其后的 enforceCodexIdentityHeaders 收口。
func ensureCodexIdentityHeaders(h http.Header) {
	if h == nil {
		return
	}
	identity := resolveCodexOutboundIdentity("")
	if strings.TrimSpace(h.Get("user-agent")) == "" {
		h.Set("user-agent", identity.userAgent)
	}
	if strings.TrimSpace(h.Get("originator")) == "" {
		h.Set("originator", identity.originator)
	}
	if strings.TrimSpace(h.Get("version")) == "" {
		h.Set("version", identity.version)
	}
	h.Set("OpenAI-Beta", "responses=experimental")
}

// applyOpenAICodexProbeHeaders 为合成探测请求补齐 Codex 身份和引擎指纹。
func applyOpenAICodexProbeHeaders(h http.Header) {
	if h == nil {
		return
	}
	ensureCodexIdentityHeaders(h)
	h.Set("X-Codex-Window-ID", uuid.NewString())
}

// enforceCodexIdentityHeaders 收口 OAuth（ChatGPT 内部接口）出站请求的客户端身份头。
// 见 enforceCodexIdentityHeadersWithUA；无账号级自定义 User-Agent 时使用本函数。
func enforceCodexIdentityHeaders(h http.Header) {
	enforceCodexIdentityHeadersWithUA(h, "")
}

// enforceCodexIdentityHeadersWithUA 强制统一 OAuth 出站身份：User-Agent / originator / version
// 一律改写为网关的规范身份，客户端自报身份不参与构造。上游在容量紧张时按客户端身份分优先级
// 降载，被降载的请求会拿到 HTTP 200 + 流内 server_is_overloaded；统一出口可确保没有请求带着
// 第三方或陈旧身份出站，也天然满足 originator 与 UA 首段配套的上游校验（issue #3901）。
//
// overrideUA 是账号级自定义 User-Agent：管理员的显式配置仍然生效，但只贡献客户端名与
// OS / 架构 / 终端指纹——版本段与 originator 都由规范身份重建，不允许出现自相矛盾或陈旧的身份。
//
// 强制统一被 gateway.disable_codex_identity_enforcement 关闭时，退回「按最终 User-Agent 配对
// originator + version 门槛校正」的收口语义，供上游策略变动时回滚。
//
// 仅对携带 originator 的请求生效：compat 桥接等非 ChatGPT 内部接口路径会显式删除 originator，
// 不应被补回。需要从缺失身份头恢复的调用方应先调用 ensureCodexIdentityHeaders。
// 必须在所有 User-Agent 改写之后调用。
func enforceCodexIdentityHeadersWithUA(h http.Header, overrideUA string) {
	if h == nil || h.Get("originator") == "" {
		return
	}
	// An account-scoped UA is an explicit routing decision even when the
	// compatibility switch disables global identity enforcement. The switch
	// only preserves the inbound client identity when no account override was
	// supplied.
	if !codexIdentityEnforcement.Load() && strings.TrimSpace(overrideUA) == "" {
		pairCodexIdentityHeaders(h)
		return
	}
	identity := resolveCodexOutboundIdentity(overrideUA)
	h.Set("user-agent", identity.userAgent)
	h.Set("originator", identity.originator)
	h.Set("version", identity.version)
}

// pairCodexIdentityHeaders 是关闭强制统一后的兜底收口：保留客户端真实身份，
// 仅保证 originator 与最终 User-Agent 首段配套、version 不低于上游门槛（issue #3901）。
func pairCodexIdentityHeaders(h http.Header) {
	originator, pairedUA, ok := openai.PairCodexClientIdentity(h.Get("user-agent"))
	if !ok {
		identity := resolveCodexOutboundIdentity("")
		originator, pairedUA = identity.originator, identity.userAgent
		h.Set("version", identity.version)
	}
	h.Set("user-agent", pairedUA)
	h.Set("originator", originator)
	if v := strings.TrimSpace(h.Get("version")); v != "" && CompareVersions(v, codexUpstreamMinVersion) < 0 {
		h.Set("version", resolveCodexOutboundIdentity("").version)
	}
}

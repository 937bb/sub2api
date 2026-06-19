package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 默认指纹值，统一使用服务端 Claude Code 默认请求头。
var defaultFingerprint = Fingerprint{
	UserAgent:               claude.DefaultHeaders["User-Agent"],
	StainlessLang:           claude.DefaultHeaders["X-Stainless-Lang"],
	StainlessPackageVersion: claude.DefaultHeaders["X-Stainless-Package-Version"],
	StainlessOS:             claude.DefaultHeaders["X-Stainless-OS"],
	StainlessArch:           claude.DefaultHeaders["X-Stainless-Arch"],
	StainlessRuntime:        claude.DefaultHeaders["X-Stainless-Runtime"],
	StainlessRuntimeVersion: claude.DefaultHeaders["X-Stainless-Runtime-Version"],
}

// Fingerprint represents account fingerprint data
type Fingerprint struct {
	ClientID                string
	UserAgent               string
	StainlessLang           string
	StainlessPackageVersion string
	StainlessOS             string
	StainlessArch           string
	StainlessRuntime        string
	StainlessRuntimeVersion string
	UpdatedAt               int64 `json:",omitempty"` // Unix timestamp，用于判断是否需要续期TTL
}

// IdentityCache defines cache operations for identity service
type IdentityCache interface {
	GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error)
	SetFingerprint(ctx context.Context, accountID int64, fp *Fingerprint) error
	// GetMaskedSessionID 获取固定的会话ID（用于会话ID伪装功能）
	// 返回的 sessionID 是一个 UUID 格式的字符串
	// 如果不存在或已过期（15分钟无请求），返回空字符串
	GetMaskedSessionID(ctx context.Context, accountID int64) (string, error)
	// SetMaskedSessionID 设置固定的会话ID，TTL 为 15 分钟
	// 每次调用都会刷新 TTL
	SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error
}

// IdentityService 管理OAuth账号的请求身份指纹
type IdentityService struct {
	cache IdentityCache
}

// NewIdentityService 创建新的IdentityService
func NewIdentityService(cache IdentityCache) *IdentityService {
	return &IdentityService{cache: cache}
}

// GetOrCreateFingerprint 获取或创建账号的服务端指纹。
// 缓存存在时仅做 24 小时 TTL 续期，不再读取客户端 UA / X-Stainless-* 回写缓存。
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*Fingerprint, error) {
	// 尝试从缓存获取指纹
	cached, err := s.cache.GetFingerprint(ctx, accountID)
	if err == nil && cached != nil {
		needWrite := time.Since(time.Unix(cached.UpdatedAt, 0)) > 24*time.Hour
		if needWrite {
			cached.UpdatedAt = time.Now().Unix()
			if err := s.cache.SetFingerprint(ctx, accountID, cached); err != nil {
				logger.LegacyPrintf("service.identity", "Warning: failed to refresh fingerprint for account %d: %v", accountID, err)
			}
		}
		return cached, nil
	}

	// 缓存不存在或解析失败，创建新指纹
	fp := s.createFingerprintFromHeaders(headers)

	// 生成随机ClientID
	fp.ClientID = generateClientID()
	fp.UpdatedAt = time.Now().Unix()

	// 保存到缓存（7天TTL，每24小时自动续期）
	if err := s.cache.SetFingerprint(ctx, accountID, fp); err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to cache fingerprint for account %d: %v", accountID, err)
	}

	logger.LegacyPrintf("service.identity", "Created new fingerprint for account %d with client_id: %s", accountID, fp.ClientID)
	return fp, nil
}

// createFingerprintFromHeaders 创建服务端默认指纹；入站客户端头不参与账号指纹缓存。
func (s *IdentityService) createFingerprintFromHeaders(_ http.Header) *Fingerprint {
	fp := defaultFingerprint
	return &fp
}

// ApplyFingerprint 将指纹应用到请求头（覆盖原有的x-stainless-*头）
// 使用 setHeaderRaw 保持原始大小写（如 X-Stainless-OS 而非 X-Stainless-Os）
func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *Fingerprint) {
	if fp == nil {
		return
	}

	// 设置user-agent
	if fp.UserAgent != "" {
		setHeaderRaw(req.Header, "User-Agent", fp.UserAgent)
	}

	// 设置x-stainless-*头（保持与 claude.DefaultHeaders 一致的大小写）
	if fp.StainlessLang != "" {
		setHeaderRaw(req.Header, "X-Stainless-Lang", fp.StainlessLang)
	}
	if fp.StainlessPackageVersion != "" {
		setHeaderRaw(req.Header, "X-Stainless-Package-Version", fp.StainlessPackageVersion)
	}
	if fp.StainlessOS != "" {
		setHeaderRaw(req.Header, "X-Stainless-OS", fp.StainlessOS)
	}
	if fp.StainlessArch != "" {
		setHeaderRaw(req.Header, "X-Stainless-Arch", fp.StainlessArch)
	}
	if fp.StainlessRuntime != "" {
		setHeaderRaw(req.Header, "X-Stainless-Runtime", fp.StainlessRuntime)
	}
	if fp.StainlessRuntimeVersion != "" {
		setHeaderRaw(req.Header, "X-Stainless-Runtime-Version", fp.StainlessRuntimeVersion)
	}
}

// RewriteUserID 重写body中的metadata.user_id
// 支持旧拼接格式和新 JSON 格式的 user_id 解析，
// 根据 fingerprintUA 版本选择输出格式。
//
// 重要：此函数使用 json.RawMessage 保留其他字段的原始字节，
// 避免重新序列化导致 thinking 块等内容被修改。
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
	}

	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return body, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return body, nil
	}

	userIDResult := metadata.Get("user_id")
	if !userIDResult.Exists() || userIDResult.Type != gjson.String {
		return body, nil
	}
	userID := userIDResult.String()
	if userID == "" {
		return body, nil
	}

	// 解析 user_id（兼容旧拼接格式和新 JSON 格式）
	parsed := ParseMetadataUserID(userID)
	if parsed == nil {
		return body, nil
	}

	sessionTail := parsed.SessionID // 原始session UUID

	// 生成新的session hash: SHA256(accountID::sessionTail) -> UUID格式
	seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
	newSessionHash := generateUUIDFromSeed(seed)

	// 根据客户端版本选择输出格式
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(cachedClientID, accountUUID, newSessionHash, version)
	if newUserID == userID {
		return body, nil
	}

	newBody, err := sjson.SetBytes(body, "metadata.user_id", newUserID)
	if err != nil {
		return body, nil
	}
	return newBody, nil
}

// RewriteUserIDWithMasking 保留旧调用名，但不再把同一账号的多个 CLI
// session 合并到一个固定 session。真实 Claude Code 每个顶层进程都有独立
// getSessionId()；Sub 只做账号级确定性派生，保持多窗口缓存边界独立。
func (s *IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	_ = ctx
	if account == nil {
		return body, nil
	}
	return s.RewriteUserID(body, account.ID, accountUUID, cachedClientID, fingerprintUA)
}

// generateRandomUUID 生成随机 UUID v4 格式字符串
func generateRandomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用时间戳生成
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		b = h[:16]
	}

	// 设置 UUID v4 版本和变体位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// generateClientID 生成64位十六进制客户端ID（32字节随机数）
func generateClientID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极罕见的情况，使用时间戳+固定值作为fallback
		logger.LegacyPrintf("service.identity", "Warning: crypto/rand.Read failed: %v, using fallback", err)
		// 使用SHA256(当前纳秒时间)作为fallback
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(h[:])
	}
	return hex.EncodeToString(b)
}

// generateUUIDFromSeed 从种子生成确定性UUID v4格式字符串
func generateUUIDFromSeed(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]

	// 设置UUID v4版本和变体位
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

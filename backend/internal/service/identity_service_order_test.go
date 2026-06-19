package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type identityCacheStub struct {
	maskedSessionID     string
	fingerprint         *Fingerprint
	setFingerprintCalls int
}

func (s *identityCacheStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return s.fingerprint, nil
}
func (s *identityCacheStub) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	copied := *fp
	s.fingerprint = &copied
	s.setFingerprintCalls++
	return nil
}
func (s *identityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return s.maskedSessionID, nil
}
func (s *identityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, sessionID string) error {
	s.maskedSessionID = sessionID
	return nil
}

func TestIdentityService_RewriteUserID_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	result, err := svc.RewriteUserID(body, 123, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, originalUserID)
	require.Contains(t, resultStr, `"metadata":{"user_id":"`)
}

func TestIdentityService_RewriteUserIDWithMasking_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{maskedSessionID: "11111111-2222-4333-8444-555555555555"}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	account := &Account{
		ID:       123,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"session_id_masking_enabled": true,
		},
	}

	result, err := svc.RewriteUserIDWithMasking(context.Background(), body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, cache.maskedSessionID)
	require.True(t, strings.Contains(resultStr, `"metadata":{"user_id":"`))
}

func TestIdentityService_RewriteUserIDWithMasking_PreservesDistinctCLISessions(t *testing.T) {
	cache := &identityCacheStub{maskedSessionID: "11111111-2222-4333-8444-555555555555"}
	svc := NewIdentityService(cache)
	account := &Account{
		ID:       123,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"session_id_masking_enabled": true,
		},
	}

	buildBody := func(sessionID string) []byte {
		userID := FormatMetadataUserID(
			"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
			"",
			sessionID,
			"2.1.78",
		)
		return []byte(`{"metadata":{"user_id":` + strconvQuote(userID) + `},"messages":[]}`)
	}

	first, err := svc.RewriteUserIDWithMasking(context.Background(), buildBody("7578cf37-aaca-46e4-a45c-71285d9dbb83"), account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	second, err := svc.RewriteUserIDWithMasking(context.Background(), buildBody("8578cf37-aaca-46e4-a45c-71285d9dbb83"), account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)

	firstParsed := ParseMetadataUserID(gjson.GetBytes(first, "metadata.user_id").String())
	secondParsed := ParseMetadataUserID(gjson.GetBytes(second, "metadata.user_id").String())
	require.NotNil(t, firstParsed)
	require.NotNil(t, secondParsed)
	require.NotEqual(t, firstParsed.SessionID, secondParsed.SessionID)
	require.NotEqual(t, cache.maskedSessionID, firstParsed.SessionID)
	require.NotEqual(t, cache.maskedSessionID, secondParsed.SessionID)
}

func TestIdentityService_CreateFingerprintFromHeadersUsesDefaultFingerprint(t *testing.T) {
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/9.9.9 (external, cli)")
	headers.Set("X-Stainless-Lang", "python")
	headers.Set("X-Stainless-Package-Version", "9.9.9")
	headers.Set("X-Stainless-OS", "Windows")

	fp := NewIdentityService(&identityCacheStub{}).createFingerprintFromHeaders(headers)

	require.Equal(t, claude.DefaultHeaders["User-Agent"], fp.UserAgent)
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Lang"], fp.StainlessLang)
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Package-Version"], fp.StainlessPackageVersion)
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], fp.StainlessOS)
}

func TestIdentityService_GetOrCreateFingerprintDoesNotMergeClientHeadersIntoCache(t *testing.T) {
	cached := &Fingerprint{
		ClientID:                "client-1",
		UserAgent:               claude.DefaultHeaders["User-Agent"],
		StainlessLang:           claude.DefaultHeaders["X-Stainless-Lang"],
		StainlessPackageVersion: claude.DefaultHeaders["X-Stainless-Package-Version"],
		StainlessOS:             claude.DefaultHeaders["X-Stainless-OS"],
		StainlessArch:           claude.DefaultHeaders["X-Stainless-Arch"],
		StainlessRuntime:        claude.DefaultHeaders["X-Stainless-Runtime"],
		StainlessRuntimeVersion: claude.DefaultHeaders["X-Stainless-Runtime-Version"],
		UpdatedAt:               time.Now().Unix(),
	}
	cache := &identityCacheStub{fingerprint: cached}
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/9.9.9 (external, cli)")
	headers.Set("X-Stainless-Lang", "python")
	headers.Set("X-Stainless-OS", "Windows")

	fp, err := NewIdentityService(cache).GetOrCreateFingerprint(context.Background(), 1, headers)

	require.NoError(t, err)
	require.Equal(t, claude.DefaultHeaders["User-Agent"], fp.UserAgent)
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Lang"], fp.StainlessLang)
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], fp.StainlessOS)
	require.Zero(t, cache.setFingerprintCalls)
}

func strconvQuote(v string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
}

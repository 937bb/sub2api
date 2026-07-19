//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type openAIPATTestOAuthClient struct{}

func (openAIPATTestOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshTokenWithOptions(ctx context.Context, refreshToken, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	return nil, nil
}

func TestHydratePersonalAccessTokenCallsOfficialWhoami(t *testing.T) {
	oldBaseURL := openAIAuthAPIBaseURL
	t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, openAIWhoamiPath, r.URL.Path)
		require.Equal(t, "Bearer at-test-pat", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Values("Accept"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                      "user@example.com",
			"chatgpt_user_id":            "user-123",
			"chatgpt_account_id":         "acc-123",
			"chatgpt_plan_type":          "plus",
			"chatgpt_account_is_fedramp": true,
		})
	}))
	defer server.Close()
	openAIAuthAPIBaseURL = server.URL

	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
	require.NoError(t, err)
	require.Equal(t, "user@example.com", metadata.Email)
	require.Equal(t, "user-123", metadata.ChatGPTUserID)
	require.Equal(t, "acc-123", metadata.ChatGPTAccountID)
	require.Equal(t, "plus", metadata.ChatGPTPlanType)
	require.True(t, metadata.ChatGPTAccountIsFedRAMP)
}

func TestHydratePersonalAccessTokenAcceptsExplicitFedRAMPFalse(t *testing.T) {
	oldBaseURL := openAIAuthAPIBaseURL
	t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, openAIWhoamiPath, r.URL.Path)
		require.Equal(t, "Bearer at-test-pat", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                      "user@example.com",
			"chatgpt_user_id":            "user-123",
			"chatgpt_account_id":         "acc-123",
			"chatgpt_plan_type":          "plus",
			"chatgpt_account_is_fedramp": false,
		})
	}))
	defer server.Close()
	openAIAuthAPIBaseURL = server.URL

	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
	require.NoError(t, err)
	require.False(t, metadata.ChatGPTAccountIsFedRAMP)
}

func TestHydratePersonalAccessTokenRequiresExplicitFedRAMPMetadata(t *testing.T) {
	tests := []struct {
		name        string
		fedRAMP     any
		wantMessage string
	}{
		{
			name:        "missing",
			wantMessage: "missing required fields: chatgpt_account_is_fedramp",
		},
		{
			name:        "null",
			fedRAMP:     nil,
			wantMessage: "missing required fields: chatgpt_account_is_fedramp",
		},
		{
			name:        "string",
			fedRAMP:     "false",
			wantMessage: "malformed metadata",
		},
		{
			name:        "number",
			fedRAMP:     0,
			wantMessage: "malformed metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldBaseURL := openAIAuthAPIBaseURL
			t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, openAIWhoamiPath, r.URL.Path)
				payload := map[string]any{
					"email":              "user@example.com",
					"chatgpt_user_id":    "user-123",
					"chatgpt_account_id": "acc-123",
					"chatgpt_plan_type":  "plus",
				}
				if tt.name != "missing" {
					payload["chatgpt_account_is_fedramp"] = tt.fedRAMP
				}
				_ = json.NewEncoder(w).Encode(payload)
			}))
			defer server.Close()
			openAIAuthAPIBaseURL = server.URL

			svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
			svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
				return req.C(), nil
			})

			metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
			require.Error(t, err)
			require.Nil(t, metadata)
			require.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsPAT(t *testing.T) {
	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body: `{"error":{"message":"denied token=at-message-token ` +
			`personal_access_token=at-kv-token Authorization=Bearer at-bearer-token","token":"at-json-token"}}`,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.NotContains(t, err, "at-message-token")
	require.NotContains(t, err, "at-kv-token")
	require.NotContains(t, err, "at-bearer-token")
	require.NotContains(t, err, "at-json-token")
	require.Contains(t, err, "personal_access_token=[redacted]")
	require.Contains(t, err, "Authorization=[redacted]")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsSensitiveMetadata(t *testing.T) {
	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body: `{"error":{"message":"Personal access token owner is inactive. email=pat-user@example.com ` +
			`chatgpt_user_id=user-sensitive chatgpt_account_id=acc-sensitive chatgpt_plan_type=enterprise ` +
			`chatgpt_account_is_fedramp=true token=hcpa-message-token","details":{"email":"pat-user@example.com",` +
			`"chatgpt_user_id":"user-sensitive","chatgpt_account_id":"acc-sensitive","chatgpt_plan_type":"enterprise",` +
			`"chatgpt_account_is_fedramp":true,"personal_access_token":"at-json-token","access_token":"hcpa-access-token",` +
			`"refresh_token":"hcpa-refresh-token","id_token":"hcpa-id-token","authorization":"Bearer hcpa-header-token"}}}`,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "Personal access token owner is inactive")
	require.Contains(t, err, `"email":"[redacted]"`)
	require.Contains(t, err, `"chatgpt_user_id":"[redacted]"`)
	require.Contains(t, err, `"chatgpt_account_id":"[redacted]"`)
	require.Contains(t, err, `"chatgpt_plan_type":"[redacted]"`)
	require.Contains(t, err, `"chatgpt_account_is_fedramp":"[redacted]"`)
	require.NotContains(t, err, "chatgpt_account_is_fedramp=true")
	require.NotContains(t, err, "pat-user@example.com")
	require.NotContains(t, err, "user-sensitive")
	require.NotContains(t, err, "acc-sensitive")
	require.NotContains(t, err, "enterprise")
	require.NotContains(t, err, "at-json-token")
	require.NotContains(t, err, "hcpa-message-token")
	require.NotContains(t, err, "hcpa-access-token")
	require.NotContains(t, err, "hcpa-refresh-token")
	require.NotContains(t, err, "hcpa-id-token")
	require.NotContains(t, err, "hcpa-header-token")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsSuffixTokenKeysInJSON(t *testing.T) {
	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       `{"error":{"message":"denied","details":{"device_token":"secret-device-token","csrf-token":"secret-csrf-token","csrfToken":"secret-camel-csrf-token","email":"pat-user@example.com","personal_access_token":"at-json-secret"}}}`,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "denied")
	require.Contains(t, err, `"device_token":"[redacted]"`)
	require.Contains(t, err, `"csrf-token":"[redacted]"`)
	require.Contains(t, err, `"csrfToken":"[redacted]"`)
	require.Contains(t, err, `"email":"[redacted]"`)
	require.Contains(t, err, `"personal_access_token":"[redacted]"`)
	require.NotContains(t, err, "secret-device-token")
	require.NotContains(t, err, "secret-csrf-token")
	require.NotContains(t, err, "secret-camel-csrf-token")
	require.NotContains(t, err, "pat-user@example.com")
	require.NotContains(t, err, "at-json-secret")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsSuffixTokenKeysInEmbeddedJSONString(t *testing.T) {
	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       `{"error":{"message":"denied","details":"{\"device_token\":\"secret-device-token\",\"csrf-token\":\"secret-csrf-token\",\"email\":\"pat-user@example.com\"}"}}`,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "denied")
	require.Contains(t, err, `\"device_token\":\"[redacted]\"`)
	require.Contains(t, err, `\"csrf-token\":\"[redacted]\"`)
	require.Contains(t, err, `\"email\":\"[redacted]\"`)
	require.NotContains(t, err, "secret-device-token")
	require.NotContains(t, err, "secret-csrf-token")
	require.NotContains(t, err, "pat-user@example.com")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsMalformedDirectSuffixTokenQuotedValues(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		secret string
	}{
		{
			name:   "underscore",
			body:   `denied device_token="secret-device-token`,
			secret: "secret-device-token",
		},
		{
			name:   "hyphen",
			body:   `denied csrf-token: "secret-csrf-token`,
			secret: "secret-csrf-token",
		},
		{
			name:   "camel",
			body:   `denied csrfToken=secret-camel-csrf-token`,
			secret: "secret-camel-csrf-token",
		},
		{
			name: "oversized",
			body: `prefix csrf-token="secret-csrf-token ` +
				strings.Repeat("x", openAIPersonalAccessTokenDiagnosticMaxBytes+64),
			secret: "secret-csrf-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&openAIPersonalAccessTokenWhoamiError{
				statusCode: http.StatusForbidden,
				body:       tt.body,
			}).Error()

			require.Contains(t, err, "OpenAI PAT whoami failed")
			require.Contains(t, err, "[redacted]")
			require.NotContains(t, err, tt.secret)
		})
	}
}

func TestOpenAIPersonalAccessTokenDiagnosticDropsMalformedDirectKVValueCrossingSensitiveBoundary(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		secret string
	}{
		{
			name:   "quoted colon boundary repro",
			input:  `denied device_token="secret-device csrf-token:"secret-csrf`,
			want:   `denied device_token=[redacted]`,
			secret: "secret-csrf",
		},
		{
			name:   "quoted equals boundary",
			input:  `denied device_token="secret-device csrf-token="secret-csrf`,
			want:   `denied device_token=[redacted]`,
			secret: "secret-csrf",
		},
		{
			name:   "single quoted suffix token boundary",
			input:  `denied device_token='secret-device workspace-token:'secret-workspace`,
			want:   `denied device_token=[redacted]`,
			secret: "secret-workspace",
		},
		{
			name:   "composite nested sensitive boundary",
			input:  `denied device_token={"csrf-token":"secret-csrf"} after`,
			want:   `denied device_token=[redacted]`,
			secret: "secret-csrf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(tt.input)

			require.Equal(t, tt.want, got)
			require.NotContains(t, got, tt.secret)
		})
	}
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsDirectSuffixTokenCompositeValues(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		secret string
	}{
		{
			name:   "object",
			input:  `whoami failed device_token={"nested":"secret-device-token","other":"safe"} after`,
			secret: "secret-device-token",
		},
		{
			name:   "array",
			input:  `whoami failed csrf-token=["secret-csrf-token"] after`,
			secret: "secret-csrf-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(tt.input)

			require.Contains(t, got, "[redacted]")
			require.Contains(t, got, "after")
			require.NotContains(t, got, tt.secret)
		})
	}
}

func TestHydratePersonalAccessTokenWhoamiErrorOversizedJSONUsesBoundedTextSanitizer(t *testing.T) {
	body := `{"error":{"message":"denied email=pat-user@example.com token=at-message-token","details":{"email":"pat-user@example.com","personal_access_token":"at-json-token","chatgpt_user_id":"user-sensitive","padding":"` +
		strings.Repeat("x", openAIPersonalAccessTokenDiagnosticMaxBytes) +
		`"}}}`

	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       body,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "denied")
	require.NotContains(t, err, "pat-user@example.com")
	require.NotContains(t, err, "at-message-token")
	require.NotContains(t, err, "at-json-token")
	require.NotContains(t, err, "user-sensitive")
	require.Contains(t, err, `"email":"[redacted]"`)
	require.Contains(t, err, `"personal_access_token":"[redacted]"`)
	require.Contains(t, err, `"chatgpt_user_id":"[redacted]"`)
}

func TestHydratePersonalAccessTokenWhoamiErrorOversizedJSONRedactsSuffixTokenKeysInFallback(t *testing.T) {
	body := `{"error":{"message":"denied","details":{"device_token":"secret-device-token","csrf-token":"secret-csrf-token","padding":"` +
		strings.Repeat("x", openAIPersonalAccessTokenDiagnosticMaxBytes) +
		`"}}}`

	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       body,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "denied")
	require.Contains(t, err, `"device_token":"[redacted]"`)
	require.Contains(t, err, `"csrf-token":"[redacted]"`)
	require.NotContains(t, err, "secret-device-token")
	require.NotContains(t, err, "secret-csrf-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsEscapedSuffixTokenKeyInOversizedFallback(t *testing.T) {
	body := `{"error":{"details":{"device\u005ftoken":"secret-device-token","padding":"` +
		strings.Repeat("x", openAIPersonalAccessTokenDiagnosticMaxBytes) +
		`"}}}`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, `"device\u005ftoken":"[redacted]"`)
	require.NotContains(t, got, "secret-device-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsEscapedSuffixTokenKeyInMalformedFallback(t *testing.T) {
	body := `{"error":{"details":{"device\u005ftoken":"secret-device-token`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, `"device\u005ftoken":"[redacted]"`)
	require.NotContains(t, got, "secret-device-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsShortMalformedDoubleEscapedUnicodeSuffixTokenKeysInEmbeddedJSONString(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		secret string
	}{
		{
			name:   "underscore",
			body:   `{"error":{"details":"{\"device\\u005ftoken\":\"secret-device-token`,
			secret: "secret-device-token",
		},
		{
			name:   "hyphen",
			body:   `{"error":{"details":"{\"csrf\\u002dtoken\":\"secret-csrf-token`,
			secret: "secret-csrf-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(tt.body)

			require.Contains(t, got, "[redacted]")
			require.NotContains(t, got, tt.secret)
		})
	}
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsRawDoubleEscapedQuotedSuffixTokenFallback(t *testing.T) {
	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(
		`prefix {\\\"device_token\\\":\\\"secret-raw-double-escaped\\\",\\\"email\\\":\\\"pat-user@example.com\\\"}`,
	)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-raw-double-escaped")
	require.NotContains(t, got, "pat-user@example.com")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsRawWhoamiShapedDiagnosticFallback(t *testing.T) {
	errText := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       `prefix {\\\"device_token\\\":\\\"secret-raw-double-escaped\\\",\\\"email\\\":\\\"pat-user@example.com\\\"}`,
	}).Error()

	require.Contains(t, errText, "OpenAI PAT whoami failed")
	require.Contains(t, errText, "[redacted]")
	require.NotContains(t, errText, "secret-raw-double-escaped")
	require.NotContains(t, errText, "pat-user@example.com")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOverlongEscapedUnderscoreSuffixTokenKeyInFallback(t *testing.T) {
	body := `{"` + strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+1) + `\u005ftoken":"secret-long-key-token`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-long-key-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOverlongEscapedHyphenSuffixTokenKeyInFallback(t *testing.T) {
	body := `{"` + strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+1) + `\u002dtoken":"secret-long-key-token`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-long-key-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOverlongDoubleEscapedUnderscoreSuffixTokenKeyInEmbeddedJSONStringFallback(t *testing.T) {
	body := `{"error":{"details":"{\"` +
		strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+64) +
		`\\u005ftoken\":\"secret-embedded-underscore`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-embedded-underscore")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOverlongDoubleEscapedHyphenSuffixTokenKeyInEmbeddedJSONStringFallback(t *testing.T) {
	body := `{"error":{"details":"{\"` +
		strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+64) +
		`\\u002dtoken\":\"secret-embedded-hyphen`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-embedded-hyphen")
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsOverlongDoubleEscapedSuffixTokenKeyInEmbeddedJSONStringFallback(t *testing.T) {
	body := `{"error":{"details":"{\"` +
		strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+64) +
		`\\u005ftoken\":\"secret-embedded-underscore`

	errText := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       body,
	}).Error()

	require.Contains(t, errText, "OpenAI PAT whoami failed")
	require.Contains(t, errText, "[redacted]")
	require.NotContains(t, errText, "secret-embedded-underscore")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOverlongMalformedEscapedSuffixTokenKeyInFallback(t *testing.T) {
	body := `{"` + strings.Repeat("a", openAIPersonalAccessTokenDiagnosticMaxKeyBytes+1) + `\u005ftoken":"secret-long-key-token", "safe_tail":`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, "[redacted]")
	require.NotContains(t, got, "secret-long-key-token")
}

func TestOpenAIPersonalAccessTokenDiagnosticRedactsOrdinaryEscapedSuffixTokenKeyInJSON(t *testing.T) {
	body := `{"device\u005ftoken":"secret-device-token","csrf\u002dtoken":"secret-csrf-token","message":"denied"}`

	got := sanitizeOpenAIPersonalAccessTokenDiagnosticText(body)

	require.Contains(t, got, `"device_token":"[redacted]"`)
	require.Contains(t, got, `"csrf-token":"[redacted]"`)
	require.Contains(t, got, `"message":"denied"`)
	require.NotContains(t, got, "secret-device-token")
	require.NotContains(t, got, "secret-csrf-token")
}

func TestHydratePersonalAccessTokenWhoamiErrorOverDepthJSONFailsClosed(t *testing.T) {
	body := `{"error":{"message":"denied","personal_access_token":` +
		strings.Repeat(`{"nested":`, openAISensitiveDiagnosticJSONMaxDepth+1) +
		`"at-deep-token"` +
		strings.Repeat(`}`, openAISensitiveDiagnosticJSONMaxDepth+1) +
		`,"safe_tail":"should-be-dropped","details":{"email":"pat-user@example.com","chatgpt_account_id":"acc-sensitive"}}}`

	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body:       body,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.Contains(t, err, "denied")
	require.NotContains(t, err, "at-deep-token")
	require.NotContains(t, err, "pat-user@example.com")
	require.NotContains(t, err, "acc-sensitive")
	require.Contains(t, err, `"personal_access_token":"[redacted]"`)
}

func TestApplyOpenAIPersonalAccessTokenMetadataPreservesOAuthCredentials(t *testing.T) {
	credentials := map[string]any{
		"personal_access_token": "at-old",
		"access_token":          "oauth-at",
		"refresh_token":         "oauth-rt",
		"id_token":              "id-token",
		"client_id":             "client-id",
	}
	metadata := &OpenAIPersonalAccessTokenMetadata{
		Email:                   "user@example.com",
		ChatGPTUserID:           "user-123",
		ChatGPTAccountID:        "acc-123",
		ChatGPTPlanType:         "pro",
		ChatGPTAccountIsFedRAMP: true,
	}

	out := ApplyOpenAIPersonalAccessTokenMetadata(credentials, "at-new", metadata)
	require.Equal(t, "at-new", out["personal_access_token"])
	require.Equal(t, "oauth-at", out["access_token"])
	require.Equal(t, "oauth-rt", out["refresh_token"])
	require.Equal(t, "id-token", out["id_token"])
	require.Equal(t, "client-id", out["client_id"])
	require.Equal(t, "user@example.com", out["email"])
	require.Equal(t, "user-123", out["chatgpt_user_id"])
	require.Equal(t, "acc-123", out["chatgpt_account_id"])
	require.Equal(t, "pro", out["chatgpt_plan_type"])
	require.Equal(t, "pro", out["plan_type"])
	require.Equal(t, true, out["chatgpt_account_is_fedramp"])
	require.NotContains(t, out, "personal_access_token_sha256")
}

func TestAccountNeedsOpenAIPersonalAccessTokenMetadataHydration(t *testing.T) {
	account := &Account{Credentials: ApplyOpenAIPersonalAccessTokenMetadata(map[string]any{}, "at-token", &OpenAIPersonalAccessTokenMetadata{
		Email:            "user@example.com",
		ChatGPTUserID:    "user-123",
		ChatGPTAccountID: "acc-123",
		ChatGPTPlanType:  "plus",
	})}
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-other"))

	account.Credentials = map[string]any{"chatgpt_account_id": "acc-123"}
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))

	account.Credentials = map[string]any{
		"chatgpt_user_id":    "user-123",
		"chatgpt_account_id": "user-123",
	}
	require.True(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))

	delete(account.Credentials, "chatgpt_account_id")
	require.True(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))
}

func TestHydratePersonalAccessTokenRequiresATPrefix(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "oauth-access-token", nil)
	require.Error(t, err)
	require.Nil(t, metadata)
	require.Contains(t, err.Error(), "must start")
}

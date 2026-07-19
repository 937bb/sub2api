package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAccountTestMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: AccountTestModeDefault},
		{input: "default", want: AccountTestModeDefault},
		{input: " compact ", want: AccountTestModeCompact},
		{input: "COMPACT", want: AccountTestModeCompact},
		{input: "unknown", want: AccountTestModeDefault},
	}

	for _, tt := range tests {
		if got := normalizeAccountTestMode(tt.input); got != tt.want {
			t.Fatalf("normalizeAccountTestMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_SuccessMarksSupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, []byte(`{"id":"cmp_1"}`), nil, now)

	if got := updates["openai_compact_supported"]; got != true {
		t.Fatalf("openai_compact_supported = %v, want true", got)
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusOK {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusOK)
	}
	if got := updates["openai_compact_last_error"]; got != "" {
		t.Fatalf("openai_compact_last_error = %v, want empty string", got)
	}
	if got := updates["openai_compact_checked_at"]; got != now.Format(time.RFC3339) {
		t.Fatalf("openai_compact_checked_at = %v, want %s", got, now.Format(time.RFC3339))
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_404MarksUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`404 page not found`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusNotFound}, body, nil, now)

	if got := updates["openai_compact_supported"]; got != false {
		t.Fatalf("openai_compact_supported = %v, want false", got)
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusNotFound {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusNotFound)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_502DoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadGateway}, []byte(`Upstream request failed`), nil, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for 502 response")
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusBadGateway {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusBadGateway)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_RequestErrorDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, errors.New("dial tcp timeout"), now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for request error")
	}
	if got, exists := updates["openai_compact_last_status"]; !exists || got != nil {
		t.Fatalf("openai_compact_last_status = %v, want nil key", got)
	}
	if got := updates["openai_compact_last_error"]; got == "" {
		t.Fatalf("expected openai_compact_last_error to be populated")
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_NoResponseClearsLastStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, nil, now)

	if got, exists := updates["openai_compact_last_status"]; !exists || got != nil {
		t.Fatalf("openai_compact_last_status = %v, want nil key", got)
	}
	if got := updates["openai_compact_last_error"]; got != "compact probe failed" {
		t.Fatalf("openai_compact_last_error = %v, want compact probe failed", got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_UnknownModelDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"message":"unknown model gpt-5.4-openai-compact"}}`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadRequest}, body, nil, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for unknown-model diagnostics")
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusBadRequest {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusBadRequest)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_EmptyFailureBodyFallsBackToHTTPStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusServiceUnavailable}, nil, nil, now)

	if got := updates["openai_compact_last_status"]; got != http.StatusServiceUnavailable {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusServiceUnavailable)
	}
	if got := updates["openai_compact_last_error"]; got != "HTTP 503" {
		t.Fatalf("openai_compact_last_error = %v, want HTTP 503", got)
	}
}

func TestOpenAIAccountTestAPIErrorMessage_RedactsPATMetadataDiagnostics(t *testing.T) {
	body := []byte(`{"error":{"message":"owner email=pat-user@example.com ` +
		`chatgpt_user_id=user-sensitive chatgpt_account_id=acc-sensitive ` +
		`chatgpt_plan_type=enterprise chatgpt_account_is_fedramp=true ` +
		`personal_access_token=at-secret"}}`)

	got := openAIAccountTestAPIErrorMessage(http.StatusUnauthorized, body)

	assertDiagnosticOmits(t, got,
		"pat-user@example.com",
		"user-sensitive",
		"acc-sensitive",
		"enterprise",
		"chatgpt_account_is_fedramp=true",
		"at-secret",
	)
	if !strings.Contains(got, "email=[redacted]") {
		t.Fatalf("diagnostic = %s, want redacted email field", got)
	}
	if !strings.Contains(got, "chatgpt_account_is_fedramp=[redacted]") {
		t.Fatalf("diagnostic = %s, want redacted non-string metadata field", got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_RedactsPATMetadataDiagnostics(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"diagnostic":{"email":"pat-user@example.com",` +
		`"chatgpt_user_id":"user-sensitive","chatgpt_account_id":"acc-sensitive",` +
		`"chatgpt_plan_type":"enterprise","chatgpt_account_is_fedramp":true,` +
		`"x-openai-fedramp":["true"],"headers":{"x-openai-fedramp":{"values":["true"]}},` +
		`"personal_access_token":"at-json-secret"}}`)

	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadGateway}, body, nil, now)
	got, _ := updates["openai_compact_last_error"].(string)

	assertDiagnosticOmits(t, got,
		"pat-user@example.com",
		"user-sensitive",
		"acc-sensitive",
		"enterprise",
		`"chatgpt_account_is_fedramp":true`,
		`"x-openai-fedramp":["true"]`,
		`"x-openai-fedramp":{"values":["true"]}`,
		"at-json-secret",
	)
	if !strings.Contains(got, `"chatgpt_account_is_fedramp":"[redacted]"`) {
		t.Fatalf("compact diagnostic = %s, want redacted non-string metadata field", got)
	}
	if !strings.Contains(got, `"x-openai-fedramp":"[redacted]"`) {
		t.Fatalf("compact diagnostic = %s, want redacted JSON array/object header field", got)
	}
	assertDiagnosticOmits(t, got, `["true"]`, `"values"`)
}

func TestSanitizeOpenAIUpstreamDiagnosticText_RedactsLongSensitiveJSONString(t *testing.T) {
	secret := strings.Repeat("a", 9000)
	got := sanitizeOpenAIUpstreamDiagnosticText(`{"access_token":"` + secret + `","other":"ok"}`)

	assertDiagnosticOmits(t, got, secret)
	if !strings.Contains(got, `"access_token":"[redacted]"`) {
		t.Fatalf("diagnostic = %s, want long access_token redacted", got)
	}
	if !strings.Contains(got, `"other":"ok"`) {
		t.Fatalf("diagnostic = %s, want non-sensitive field preserved", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForOverlongSensitiveJSONArray(t *testing.T) {
	secret := "secret-array-token"
	diagnosticTail := `"safe_tail":"should-be-dropped"}`
	input := `{"access_token":["` + secret + `","` + strings.Repeat("x", openAISensitiveDiagnosticJSONCompositeMaxScan+1) + `"],` + diagnosticTail

	got := sanitizeOpenAIUpstreamDiagnosticText(input)

	assertDiagnosticOmits(t, got, secret, diagnosticTail, "should-be-dropped", strings.Repeat("x", 64))
	if !strings.Contains(got, `"access_token":"[redacted]"`) {
		t.Fatalf("diagnostic = %s, want overlong access_token array redacted", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForTooDeepSensitiveJSONObject(t *testing.T) {
	secret := "secret-object-token"
	input := `{"access_token":` + strings.Repeat(`{"nested":`, openAISensitiveDiagnosticJSONMaxDepth+1) + `"` + secret + `"` + strings.Repeat(`}`, openAISensitiveDiagnosticJSONMaxDepth+1) + `,"other":"should-be-dropped"}`

	got := sanitizeOpenAIUpstreamDiagnosticText(input)

	assertDiagnosticOmits(t, got, secret, `"other":"should-be-dropped"`)
	if !strings.Contains(got, `"access_token":"[redacted]"`) {
		t.Fatalf("diagnostic = %s, want over-depth access_token object redacted", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForMalformedSensitiveJSONScalar(t *testing.T) {
	tests := []struct {
		name  string
		input string
		leaks []string
	}{
		{
			name:  "truncated quoted scalar",
			input: `{"access_token":"secret-token`,
			leaks: []string{"secret-token"},
		},
		{
			name:  "unknown unquoted scalar",
			input: `{"access_token":secret-token,"other":"ok"}`,
			leaks: []string{"secret-token"},
		},
		{
			name:  "number prefix scalar",
			input: `{"access_token":123secret}`,
			leaks: []string{"123secret", "secret"},
		},
		{
			name:  "true prefix scalar",
			input: `{"access_token":true-secret}`,
			leaks: []string{"true-secret", "secret"},
		},
		{
			name:  "false prefix scalar",
			input: `{"access_token":false-secret}`,
			leaks: []string{"false-secret", "secret"},
		},
		{
			name:  "null prefix scalar",
			input: `{"access_token":null-secret}`,
			leaks: []string{"null-secret", "secret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIUpstreamDiagnosticText(tt.input)

			assertDiagnosticOmits(t, got, tt.leaks...)
			if !strings.Contains(got, `"access_token":"[redacted]"`) {
				t.Fatalf("diagnostic = %s, want malformed access_token redacted", got)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForMalformedSensitiveKV(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		leaks        []string
		wantContains string
	}{
		{
			name:         "truncated unescaped quoted kv",
			input:        `owner access_token="secret-token`,
			leaks:        []string{"secret-token"},
			wantContains: "access_token=[redacted]",
		},
		{
			name:         "truncated escaped quoted kv drops unsafe tail",
			input:        `owner access_token=\"secret-token personal_access_token=\"secret-pat`,
			leaks:        []string{"secret-token", "secret-pat", "personal_access_token"},
			wantContains: "access_token=[redacted]",
		},
		{
			name:         "bearer value without token",
			input:        `authorization=Bearer `,
			wantContains: "authorization=[redacted]",
		},
		{
			name:         "bearer tab without token",
			input:        "access_token=Bearer\t",
			wantContains: "access_token=[redacted]",
		},
		{
			name:         "double quoted key with equals",
			input:        `{"access_token"="secret-token"}`,
			leaks:        []string{"secret-token"},
			wantContains: `"access_token"=[redacted]`,
		},
		{
			name:         "single quoted key and value with colon",
			input:        `{'access_token':'secret-token'}`,
			leaks:        []string{"secret-token"},
			wantContains: `'access_token':[redacted]`,
		},
		{
			name:         "escaped double quoted key with equals",
			input:        `{\"access_token\"=\"secret-token\"}`,
			leaks:        []string{"secret-token"},
			wantContains: `\"access_token\"=[redacted]`,
		},
		{
			name:         "escaped single quoted access token key with equals",
			input:        `owner \'access_token\'=\'secret-token\' suffix`,
			leaks:        []string{"secret-token"},
			wantContains: `\'access_token\'=[redacted]`,
		},
		{
			name:         "escaped single quoted personal access token key with equals",
			input:        `owner \'personal_access_token\'=\'secret-pat\' suffix`,
			leaks:        []string{"secret-pat"},
			wantContains: `\'personal_access_token\'=[redacted]`,
		},
		{
			name:         "escaped single quoted value with escaped quote content",
			input:        `owner access_token=\\'prefix\\\\' secret-tail-overlay\\' suffix`,
			leaks:        []string{"secret-tail-overlay"},
			wantContains: `access_token=[redacted] suffix`,
		},
		{
			name:         "escaped double quoted value with escaped quote content",
			input:        `owner personal_access_token=\\"prefix\\\\" secret-tail-overlay\\" suffix`,
			leaks:        []string{"secret-tail-overlay"},
			wantContains: `personal_access_token=[redacted] suffix`,
		},
		{
			name:         "double escaped double quoted access token key with equals",
			input:        `prefix \\\"access_token\\\"=\\\"secret-token\\\" suffix`,
			leaks:        []string{"secret-token"},
			wantContains: `\\\"access_token\\\"=[redacted]`,
		},
		{
			name:         "raw two-backslash double quoted access token key with equals",
			input:        `owner \\"access_token\\"=\\"secret-token\\" suffix`,
			leaks:        []string{"secret-token"},
			wantContains: `\\"access_token\\"=[redacted]`,
		},
		{
			name:         "double escaped double quoted personal access token key with equals",
			input:        `prefix \\\"personal_access_token\\\"=\\\"secret-pat\\\" suffix`,
			leaks:        []string{"secret-pat"},
			wantContains: `\\\"personal_access_token\\\"=[redacted]`,
		},
		{
			name:         "raw two-backslash double quoted personal access token key with equals",
			input:        `owner \\"personal_access_token\\"=\\"secret-pat\\" suffix`,
			leaks:        []string{"secret-pat"},
			wantContains: `\\"personal_access_token\\"=[redacted]`,
		},
		{
			name:         "escaped double quoted key with colon",
			input:        `{\"personal_access_token\":\"secret-pat\"}`,
			leaks:        []string{"secret-pat"},
			wantContains: `\"personal_access_token\":\"[redacted]\"`,
		},
		{
			name:         "query bearer value includes token suffix",
			input:        `https://upstream.example/fail?access_token=Bearer secret-token&safe=ok`,
			leaks:        []string{"secret-token"},
			wantContains: `access_token=[redacted]&safe=ok`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIUpstreamDiagnosticText(tt.input)

			assertDiagnosticOmits(t, got, tt.leaks...)
			if !strings.Contains(got, tt.wantContains) {
				t.Fatalf("diagnostic = %s, want %s", got, tt.wantContains)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_RedactsEscapedQuotedKVValueWithEscapedQuoteContent(t *testing.T) {
	message := `owner personal_access_token=\\'prefix\\\\' secret-tail-overlay\\' suffix`
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
		},
	})
	if err != nil {
		t.Fatalf("marshal diagnostic body: %v", err)
	}

	got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)
	assertDiagnosticOmits(t, got, "secret-tail-overlay")

	var decoded struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
	}
	assertDiagnosticOmits(t, decoded.Error.Message, "secret-tail-overlay")
	if !strings.Contains(decoded.Error.Message, `personal_access_token=[redacted] suffix`) {
		t.Fatalf("message = %s, want escaped quoted KV value fully redacted", decoded.Error.Message)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_RedactsEscapedSingleQuotedKVMessage(t *testing.T) {
	tests := []struct {
		key    string
		secret string
	}{
		{key: "access_token", secret: "secret-token"},
		{key: "personal_access_token", secret: "secret-pat"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			message := `owner \'` + tt.key + `\'=\'` + tt.secret + `\' suffix`
			body, err := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": message,
				},
			})
			if err != nil {
				t.Fatalf("marshal diagnostic body: %v", err)
			}

			got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)
			assertDiagnosticOmits(t, got, tt.secret)

			var decoded struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
			}
			assertDiagnosticOmits(t, decoded.Error.Message, tt.secret)
			want := `\'` + tt.key + `\'=[redacted]`
			if !strings.Contains(decoded.Error.Message, want) {
				t.Fatalf("message = %s, want escaped single quoted KV redacted as %s", decoded.Error.Message, want)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForExcessiveKVWhitespace(t *testing.T) {
	unsafeTail := "personal_access_token=secret-pat"
	got := sanitizeOpenAIUpstreamDiagnosticText("access_token=" + strings.Repeat(" ", openAISensitiveDiagnosticKVValueMaxScan+1) + unsafeTail)

	assertDiagnosticOmits(t, got, unsafeTail, "secret-pat", strings.Repeat(" ", 64))
	if got != "access_token=[redacted]" {
		t.Fatalf("diagnostic = %q, want fail-closed redaction without copied whitespace", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForExcessiveKVWhitespaceBeforeSeparator(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		separator string
		secret    string
	}{
		{
			name:      "access token equals",
			key:       "access_token",
			separator: "=",
			secret:    "secret-token",
		},
		{
			name:      "personal access token colon",
			key:       "personal_access_token",
			separator: ":",
			secret:    "secret-pat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			whitespace := strings.Repeat(" ", openAISensitiveDiagnosticKVValueMaxScan+1)
			got := sanitizeOpenAIUpstreamDiagnosticText("owner " + tt.key + whitespace + tt.separator + tt.secret)

			assertDiagnosticOmits(t, got, whitespace, strings.Repeat(" ", 64), tt.separator+tt.secret, tt.secret)
			want := "owner " + tt.key + "[redacted]"
			if got != want {
				t.Fatalf("diagnostic = %q, want fail-closed redaction %q", got, want)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForExcessiveKVBackslashRun(t *testing.T) {
	unsafeTail := "x personal_access_token=secret-pat"
	got := sanitizeOpenAIUpstreamDiagnosticText("access_token=" + strings.Repeat("\\", openAISensitiveDiagnosticKVValueMaxScan+1) + unsafeTail)

	assertDiagnosticOmits(t, got, unsafeTail, "secret-pat", strings.Repeat("\\", 64))
	if got != "access_token=[redacted]" {
		t.Fatalf("diagnostic = %q, want fail-closed redaction without copied backslash run", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticText_FailsClosedForOverlongQuotedKVKeyClose(t *testing.T) {
	backslashRun := strings.Repeat("\\", openAISensitiveDiagnosticKVValueMaxScan+1)
	tests := []struct {
		name   string
		opener string
		key    string
		secret string
	}{
		{
			name:   "ordinary quoted access token key",
			opener: `"`,
			key:    "access_token",
			secret: "secret-token",
		},
		{
			name:   "escaped quoted access token key",
			opener: `\"`,
			key:    "access_token",
			secret: "secret-token",
		},
		{
			name:   "ordinary quoted personal access token key",
			opener: `"`,
			key:    "personal_access_token",
			secret: "secret-pat",
		},
		{
			name:   "escaped quoted personal access token key",
			opener: `\"`,
			key:    "personal_access_token",
			secret: "secret-pat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "owner " + tt.opener + tt.key + backslashRun + `"=` + tt.secret + " suffix"
			got := sanitizeOpenAIUpstreamDiagnosticText(input)

			assertDiagnosticOmits(t, got, tt.secret, strings.Repeat("\\", 64), `"=`+tt.secret, "suffix")
			want := "owner " + tt.opener + tt.key + "[redacted]"
			if got != want {
				t.Fatalf("diagnostic = %q, want fail-closed redaction %q", got, want)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_FailsClosedForOverlongQuotedKVKeyClose(t *testing.T) {
	backslashRun := strings.Repeat("\\", openAISensitiveDiagnosticKVValueMaxScan+1)
	tests := []struct {
		key    string
		secret string
	}{
		{key: "access_token", secret: "secret-token"},
		{key: "personal_access_token", secret: "secret-pat"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			input := `owner \"` + tt.key + backslashRun + `"=` + tt.secret + " suffix"
			body, err := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": input,
				},
			})
			if err != nil {
				t.Fatalf("marshal diagnostic body: %v", err)
			}

			got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)
			assertDiagnosticOmits(t, got, tt.secret)

			var decoded struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
			}
			assertDiagnosticOmits(t, decoded.Error.Message, tt.secret, strings.Repeat("\\", 64), `"=`+tt.secret, "suffix")
			want := `owner \"` + tt.key + "[redacted]"
			if decoded.Error.Message != want {
				t.Fatalf("message = %q, want fail-closed redaction %q", decoded.Error.Message, want)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_RedactsRawTwoBackslashEscapedKVMessage(t *testing.T) {
	validBody, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": `owner \\"access_token\\"=\\"secret-token\\" suffix`,
		},
	})
	if err != nil {
		t.Fatalf("marshal diagnostic body: %v", err)
	}

	t.Run("valid json string sanitizer", func(t *testing.T) {
		got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(validBody, 4096)

		assertDiagnosticOmits(t, got, "secret-token")
		var outer map[string]any
		if err := json.Unmarshal([]byte(got), &outer); err != nil {
			t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
		}
		errorObject, ok := outer["error"].(map[string]any)
		if !ok {
			t.Fatalf("sanitized body error field = %#v, want object", outer["error"])
		}
		message, ok := errorObject["message"].(string)
		if !ok {
			t.Fatalf("sanitized body error.message = %#v, want string", errorObject["message"])
		}
		assertDiagnosticOmits(t, message, "secret-token")
		if !strings.Contains(message, `\\"access_token\\"=[redacted]`) {
			t.Fatalf("message = %s, want raw two-backslash access_token redacted", message)
		}
	})

	t.Run("raw fallback reviewer spelling", func(t *testing.T) {
		body := []byte(`{"error":{"message":"owner \\\\"access_token\\\\"=\\\\"secret-token\\\\" suffix"}}`)
		got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)

		assertDiagnosticOmits(t, got, "secret-token")
		if !strings.Contains(got, `\\\\"access_token\\\\"=[redacted]`) {
			t.Fatalf("diagnostic = %s, want raw fallback access_token redacted", got)
		}
	})
}

func TestSanitizeOpenAIUpstreamDiagnostic_RedactsEscapedQuotedKVBackslashRuns(t *testing.T) {
	fields := []struct {
		key    string
		secret string
	}{
		{key: "access_token", secret: "secret-token"},
		{key: "personal_access_token", secret: "secret-pat"},
	}

	for _, field := range fields {
		for _, backslashes := range []int{1, 2, 3} {
			name := field.key + "/" + strings.Repeat("backslash-", backslashes) + "quote"
			t.Run(name, func(t *testing.T) {
				input := escapedQuotedSensitiveKVRuntime(backslashes, field.key, field.secret)
				want := escapedQuoteRuntime(backslashes) + field.key + escapedQuoteRuntime(backslashes) + "=[redacted]"

				gotText := sanitizeOpenAIUpstreamDiagnosticText(input)
				assertDiagnosticOmits(t, gotText, field.secret)
				if !strings.Contains(gotText, want) {
					t.Fatalf("diagnostic text = %q, want %q", gotText, want)
				}

				body, err := json.Marshal(map[string]any{
					"error": map[string]any{
						"message": input,
					},
				})
				if err != nil {
					t.Fatalf("marshal diagnostic body: %v", err)
				}

				gotBody := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)
				assertDiagnosticOmits(t, gotBody, field.secret)

				var decoded struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(gotBody), &decoded); err != nil {
					t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, gotBody)
				}
				assertDiagnosticOmits(t, decoded.Error.Message, field.secret)
				if !strings.Contains(decoded.Error.Message, want) {
					t.Fatalf("diagnostic body message = %q, want %q", decoded.Error.Message, want)
				}
			})
		}
	}
}

func escapedQuotedSensitiveKVRuntime(backslashes int, key, value string) string {
	quote := escapedQuoteRuntime(backslashes)
	return "owner " + quote + key + quote + "=" + quote + value + quote + " suffix"
}

func escapedQuoteRuntime(backslashes int) string {
	return strings.Repeat("\\", backslashes) + `"`
}

func TestSanitizeOpenAIUpstreamDiagnostic_RedactsPrefixedUnicodeEscapedEmbeddedJSONMessage(t *testing.T) {
	tests := []struct {
		key          string
		escapedKey   string
		secret       string
		wantContains string
	}{
		{
			key:          "access_token",
			escapedKey:   `access\u005ftoken`,
			secret:       "secret-token",
			wantContains: `"access\u005ftoken":"[redacted]"`,
		},
		{
			key:          "personal_access_token",
			escapedKey:   `personal\u005faccess\u005ftoken`,
			secret:       "secret-pat",
			wantContains: `"personal\u005faccess\u005ftoken":"[redacted]"`,
		},
	}

	for _, tt := range tests {
		t.Run("text/"+tt.key, func(t *testing.T) {
			input := `bad {"` + tt.escapedKey + `":"` + tt.secret + `"} suffix`
			got := sanitizeOpenAIUpstreamDiagnosticText(input)

			assertDiagnosticOmits(t, got, tt.secret)
			if !strings.Contains(got, tt.wantContains) {
				t.Fatalf("diagnostic text = %s, want %s", got, tt.wantContains)
			}
			if !strings.Contains(got, "bad ") || !strings.Contains(got, " suffix") {
				t.Fatalf("diagnostic text = %s, want prefixed and suffixed text preserved", got)
			}
		})

		t.Run("json body message/"+tt.key, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": `bad {"` + tt.escapedKey + `":"` + tt.secret + `"} suffix`,
				},
			})
			if err != nil {
				t.Fatalf("marshal diagnostic body: %v", err)
			}

			got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)
			assertDiagnosticOmits(t, got, tt.secret)

			var decoded struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
			}
			assertDiagnosticOmits(t, decoded.Error.Message, tt.secret)
			if !strings.Contains(decoded.Error.Message, tt.wantContains) {
				t.Fatalf("diagnostic body message = %s, want %s", decoded.Error.Message, tt.wantContains)
			}
			if !strings.Contains(decoded.Error.Message, "bad ") || !strings.Contains(decoded.Error.Message, " suffix") {
				t.Fatalf("diagnostic body message = %s, want prefixed and suffixed text preserved", decoded.Error.Message)
			}
		})
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_RedactsEscapedEmbeddedJSONMessage(t *testing.T) {
	embedded := `{"access_token":"secret-access","personal_access_token":"secret-pat",` +
		`"x-openai-fedramp":"fedramp-scalar","array":{"x-openai-fedramp":["fedramp-array"]},` +
		`"object":{"x-openai-fedramp":{"value":"fedramp-object"}},"safe":"ok"}`
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": embedded,
		},
	})
	if err != nil {
		t.Fatalf("marshal diagnostic body: %v", err)
	}

	got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(body, 4096)

	assertDiagnosticOmits(t, got, "secret-access", "secret-pat", "fedramp-scalar", "fedramp-array", "fedramp-object")
	var outer map[string]any
	if err := json.Unmarshal([]byte(got), &outer); err != nil {
		t.Fatalf("sanitized body is not valid JSON: %v; body=%s", err, got)
	}
	errorObject, ok := outer["error"].(map[string]any)
	if !ok {
		t.Fatalf("sanitized body error field = %#v, want object", outer["error"])
	}
	message, ok := errorObject["message"].(string)
	if !ok {
		t.Fatalf("sanitized body error.message = %#v, want string", errorObject["message"])
	}
	assertDiagnosticOmits(t, message,
		"secret-access",
		"secret-pat",
		"fedramp-scalar",
		"fedramp-array",
		"fedramp-object",
	)

	var inner map[string]any
	if err := json.Unmarshal([]byte(message), &inner); err != nil {
		t.Fatalf("sanitized embedded message is not valid JSON: %v; message=%s", err, message)
	}
	if got := inner["access_token"]; got != "[redacted]" {
		t.Fatalf("embedded access_token = %#v, want redacted", got)
	}
	if got := inner["personal_access_token"]; got != "[redacted]" {
		t.Fatalf("embedded personal_access_token = %#v, want redacted", got)
	}
	if got := inner["x-openai-fedramp"]; got != "[redacted]" {
		t.Fatalf("embedded x-openai-fedramp scalar = %#v, want redacted", got)
	}
	if got := inner["array"].(map[string]any)["x-openai-fedramp"]; got != "[redacted]" {
		t.Fatalf("embedded x-openai-fedramp array = %#v, want redacted", got)
	}
	if got := inner["object"].(map[string]any)["x-openai-fedramp"]; got != "[redacted]" {
		t.Fatalf("embedded x-openai-fedramp object = %#v, want redacted", got)
	}
	if got := inner["safe"]; got != "ok" {
		t.Fatalf("embedded safe field = %#v, want preserved", got)
	}
}

func TestSanitizeOpenAIUpstreamDiagnosticBodyForLog_FailsClosedForMalformedEscapedEmbeddedJSON(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		wantContains []string
	}{
		{
			name: "truncated escaped scalar",
			body: []byte(`{"error":{"message":"bad {\"access_token\":\"secret-access`),
			wantContains: []string{
				`\"access_token\":\"[redacted]\"`,
			},
		},
		{
			name: "malformed escaped fields",
			body: []byte(`{"error":{"message":"bad {\"access_token\":\"secret-access\",\"personal_access_token\":\"secret-pat\",\"x-openai-fedramp\":\"fedramp-scalar\",\"array\":{\"x-openai-fedramp\":[\"fedramp-array\"]},\"object\":{\"x-openai-fedramp\":{\"value\":\"fedramp-object\"}},\"safe\":\"ok"`),
			wantContains: []string{
				`\"access_token\":\"[redacted]\"`,
				`\"personal_access_token\":\"[redacted]\"`,
				`\"x-openai-fedramp\":\"[redacted]\"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOpenAIUpstreamDiagnosticBodyForLog(tt.body, 4096)

			assertDiagnosticOmits(t, got,
				"secret-access",
				"secret-pat",
				"fedramp-scalar",
				"fedramp-array",
				"fedramp-object",
			)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Fatalf("diagnostic = %s, want %s", got, want)
				}
			}
		})
	}
}

func assertDiagnosticOmits(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			t.Fatalf("diagnostic leaked %q in %s", fragment, text)
		}
	}
}

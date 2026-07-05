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

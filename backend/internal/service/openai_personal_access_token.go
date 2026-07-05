package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

const (
	openAIPersonalAccessTokenPrefix                    = "at-"
	defaultOpenAIAuthAPIBaseURL                        = "https://auth.openai.com/api/accounts"
	openAIAuthAPIBaseURLEnvVar                         = "CODEX_AUTHAPI_BASE_URL"
	openAIWhoamiPath                                   = "/v1/user-auth-credential/whoami"
	openAIPersonalAccessTokenDiagnosticMaxBytes        = openAISensitiveDiagnosticJSONBodyMaxParse
	openAIPersonalAccessTokenDiagnosticMaxKeyBytes     = 512
	openAIPersonalAccessTokenDiagnosticRawKeyTailBytes = openAIPersonalAccessTokenDiagnosticMaxKeyBytes
)

var openAIAuthAPIBaseURL = defaultOpenAIAuthAPIBaseURL

var (
	openAIPersonalAccessTokenDiagnosticFieldPattern             = `personal_access_token|access_token|refresh_token|id_token|session_token|authorization|api_key|apikey|token|email|chatgpt_user_id|chatgpt_account_id|chatgpt_plan_type|chatgpt_account_is_fedramp|[A-Za-z0-9_.-]+(?:_token|-token|token)`
	openAIPersonalAccessTokenDiagnosticJSONFieldRe              = regexp.MustCompile(`(?i)("(?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)"\s*:\s*)(?:"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)`)
	openAIPersonalAccessTokenDiagnosticJSONKeyRe                = regexp.MustCompile(`(?i)"(?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)"\s*:\s*`)
	openAIPersonalAccessTokenDiagnosticEscapedJSONKeyRe         = regexp.MustCompile(`(?i)\\"(?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)\\"\s*:\s*`)
	openAIPersonalAccessTokenDiagnosticKVKeyRe                  = regexp.MustCompile(`(?i)\b(?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)\s*(?:=|:)\s*`)
	openAIPersonalAccessTokenDiagnosticEmailRe                  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	openAIPersonalAccessTokenDiagnosticValueRe                  = regexp.MustCompile(`\bat-[A-Za-z0-9._~+/=-]+`)
	openAIPersonalAccessTokenDiagnosticRawSensitiveKeyFragments = []string{
		"access_token",
		"personal_access_token",
		"refresh_token",
		"session_token",
		"authorization",
		"api_key",
		"apikey",
		"email",
		"chatgpt_user_id",
		"chatgpt_account_id",
		"chatgpt_plan_type",
		"chatgpt_account_is_fedramp",
	}
)

// OpenAIPersonalAccessTokenMetadata is the official Codex whoami response for
// personal access tokens.
type OpenAIPersonalAccessTokenMetadata struct {
	Email                   string `json:"email"`
	ChatGPTUserID           string `json:"chatgpt_user_id"`
	ChatGPTAccountID        string `json:"chatgpt_account_id"`
	ChatGPTPlanType         string `json:"chatgpt_plan_type"`
	ChatGPTAccountIsFedRAMP bool   `json:"chatgpt_account_is_fedramp"`
}

type openAIPersonalAccessTokenWhoamiMetadata struct {
	Email                   string `json:"email"`
	ChatGPTUserID           string `json:"chatgpt_user_id"`
	ChatGPTAccountID        string `json:"chatgpt_account_id"`
	ChatGPTPlanType         string `json:"chatgpt_plan_type"`
	ChatGPTAccountIsFedRAMP *bool  `json:"chatgpt_account_is_fedramp"`
}

type openAIPersonalAccessTokenWhoamiError struct {
	statusCode int
	body       string
}

func (e *openAIPersonalAccessTokenWhoamiError) Error() string {
	if e == nil {
		return "OpenAI PAT whoami failed"
	}
	body := truncateOpenAIWhoamiBody(sanitizeOpenAIPersonalAccessTokenDiagnosticText(e.body))
	return fmt.Sprintf("OpenAI PAT whoami failed: status %d, body: %s", e.statusCode, body)
}

// ValidateOpenAIPersonalAccessToken applies Codex's PAT classification rule.
func ValidateOpenAIPersonalAccessToken(personalAccessToken string) error {
	if strings.TrimSpace(personalAccessToken) == "" {
		return fmt.Errorf("personal_access_token is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(personalAccessToken), openAIPersonalAccessTokenPrefix) {
		return fmt.Errorf("personal_access_token must start with %q", openAIPersonalAccessTokenPrefix)
	}
	return nil
}

// AccountNeedsOpenAIPersonalAccessTokenMetadataHydration reports whether the
// account lacks the ChatGPT account id needed to route PAT-backed requests.
func AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account *Account, personalAccessToken string) bool {
	if account == nil || account.Credentials == nil {
		return true
	}
	accountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if accountID == "" {
		return true
	}
	// Older records may have persisted the ChatGPT user id in the account-id
	// slot. Hydrate once so PAT requests carry the workspace/account id instead.
	return strings.HasPrefix(accountID, "user-")
}

// BuildOpenAIPersonalAccessTokenCredentialUpdates converts whoami metadata into
// account credential fields used by the Codex request path.
func BuildOpenAIPersonalAccessTokenCredentialUpdates(personalAccessToken string, metadata *OpenAIPersonalAccessTokenMetadata) map[string]any {
	updates := map[string]any{
		"personal_access_token_hydrated_at": time.Now().UTC().Format(time.RFC3339),
		"chatgpt_account_is_fedramp":        false,
	}
	if strings.TrimSpace(personalAccessToken) != "" {
		updates["personal_access_token"] = strings.TrimSpace(personalAccessToken)
	}
	if metadata == nil {
		return updates
	}
	updates["chatgpt_account_is_fedramp"] = metadata.ChatGPTAccountIsFedRAMP
	if value := strings.TrimSpace(metadata.Email); value != "" {
		updates["email"] = value
	}
	if value := strings.TrimSpace(metadata.ChatGPTUserID); value != "" {
		updates["chatgpt_user_id"] = value
	}
	if value := strings.TrimSpace(metadata.ChatGPTAccountID); value != "" {
		updates["chatgpt_account_id"] = value
	}
	if value := strings.TrimSpace(metadata.ChatGPTPlanType); value != "" {
		updates["chatgpt_plan_type"] = value
		updates["plan_type"] = value
	}
	return updates
}

// ApplyOpenAIPersonalAccessTokenMetadata returns a credentials map with whoami
// metadata merged in, without dropping existing OAuth AT/RT credentials.
func ApplyOpenAIPersonalAccessTokenMetadata(credentials map[string]any, personalAccessToken string, metadata *OpenAIPersonalAccessTokenMetadata) map[string]any {
	out := cloneCredentials(credentials)
	for key, value := range BuildOpenAIPersonalAccessTokenCredentialUpdates(personalAccessToken, metadata) {
		out[key] = value
	}
	return out
}

func persistOpenAIPersonalAccessTokenMetadata(ctx context.Context, repo AccountRepository, account *Account, personalAccessToken string, metadata *OpenAIPersonalAccessTokenMetadata) error {
	if account == nil {
		return nil
	}
	updates := BuildOpenAIPersonalAccessTokenCredentialUpdates(personalAccessToken, metadata)
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	for key, value := range updates {
		account.Credentials[key] = value
	}
	if repo == nil || account.ID <= 0 {
		return nil
	}
	// Use the single-account credential path so scheduler snapshots refresh
	// immediately after fixing stale PAT metadata.
	return persistAccountCredentials(ctx, repo, account, account.Credentials)
}

// HydratePersonalAccessToken calls the official Codex whoami endpoint for PATs:
// GET https://auth.openai.com/api/accounts/v1/user-auth-credential/whoami
// Authorization: Bearer <personal_access_token>
func (s *OpenAIOAuthService) HydratePersonalAccessToken(ctx context.Context, personalAccessToken string, proxyID *int64) (*OpenAIPersonalAccessTokenMetadata, error) {
	personalAccessToken = strings.TrimSpace(personalAccessToken)
	if err := ValidateOpenAIPersonalAccessToken(personalAccessToken); err != nil {
		return nil, err
	}

	proxyURL, err := s.openAIPersonalAccessTokenProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	client, err := s.openAIPersonalAccessTokenClient(proxyURL)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := client.R().
		SetContext(reqCtx).
		SetHeader("Authorization", "Bearer "+personalAccessToken).
		Get(openAIWhoamiURL())
	if err != nil {
		return nil, fmt.Errorf("OpenAI PAT whoami request failed: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("OpenAI PAT whoami request returned no response")
	}
	if !resp.IsSuccessState() {
		return nil, &openAIPersonalAccessTokenWhoamiError{statusCode: resp.StatusCode, body: sanitizeOpenAIPersonalAccessTokenDiagnosticText(resp.String())}
	}
	var metadata openAIPersonalAccessTokenWhoamiMetadata
	if err := json.Unmarshal(resp.Bytes(), &metadata); err != nil {
		return nil, fmt.Errorf("OpenAI PAT whoami returned malformed metadata: %w", err)
	}
	if err := metadata.validate(); err != nil {
		return nil, err
	}
	return metadata.toPublic(), nil
}

func (m *openAIPersonalAccessTokenWhoamiMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("OpenAI PAT whoami returned empty metadata")
	}
	missing := make([]string, 0, 5)
	if strings.TrimSpace(m.Email) == "" {
		missing = append(missing, "email")
	}
	if strings.TrimSpace(m.ChatGPTUserID) == "" {
		missing = append(missing, "chatgpt_user_id")
	}
	if strings.TrimSpace(m.ChatGPTAccountID) == "" {
		missing = append(missing, "chatgpt_account_id")
	}
	if strings.TrimSpace(m.ChatGPTPlanType) == "" {
		missing = append(missing, "chatgpt_plan_type")
	}
	if m.ChatGPTAccountIsFedRAMP == nil {
		missing = append(missing, "chatgpt_account_is_fedramp")
	}
	if len(missing) > 0 {
		return fmt.Errorf("OpenAI PAT whoami metadata missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (m *openAIPersonalAccessTokenWhoamiMetadata) toPublic() *OpenAIPersonalAccessTokenMetadata {
	if m == nil {
		return nil
	}
	return &OpenAIPersonalAccessTokenMetadata{
		Email:                   m.Email,
		ChatGPTUserID:           m.ChatGPTUserID,
		ChatGPTAccountID:        m.ChatGPTAccountID,
		ChatGPTPlanType:         m.ChatGPTPlanType,
		ChatGPTAccountIsFedRAMP: *m.ChatGPTAccountIsFedRAMP,
	}
}

func (s *OpenAIOAuthService) openAIPersonalAccessTokenProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || *proxyID == 0 {
		return "", nil
	}
	if s == nil || s.proxyRepo == nil {
		return "", fmt.Errorf("proxy repository is not configured")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", err
	}
	if proxy == nil {
		return "", fmt.Errorf("proxy not found")
	}
	return proxy.URL(), nil
}

func (s *OpenAIOAuthService) openAIPersonalAccessTokenClient(proxyURL string) (*req.Client, error) {
	if s != nil && s.privacyClientFactory != nil {
		return s.privacyClientFactory(proxyURL)
	}
	if strings.TrimSpace(proxyURL) != "" {
		return nil, fmt.Errorf("OpenAI PAT whoami proxy client is not configured")
	}
	return req.C().SetTimeout(30 * time.Second), nil
}

func openAIWhoamiURL() string {
	baseURL := strings.TrimSpace(os.Getenv(openAIAuthAPIBaseURLEnvVar))
	if baseURL == "" {
		baseURL = openAIAuthAPIBaseURL
	}
	if baseURL == "" {
		baseURL = defaultOpenAIAuthAPIBaseURL
	}
	return strings.TrimRight(baseURL, "/") + openAIWhoamiPath
}

func truncateOpenAIWhoamiBody(body string) string {
	body = strings.TrimSpace(body)
	const limit = 1024
	if len(body) <= limit {
		return body
	}
	return body[:limit] + "..."
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticText(text string) string {
	if text == "" {
		return text
	}
	text = sanitizeOpenAIPersonalAccessTokenDiagnosticJSON(text)
	return sanitizeOpenAIPersonalAccessTokenDiagnosticFields(text)
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticFields(text string) string {
	if text == "" {
		return text
	}
	text = redactOpenAIPersonalAccessTokenDiagnosticKVFields(text)
	text = sanitizeOpenAIUpstreamDiagnosticText(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticDecodedJSONFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticLayeredEscapedJSONFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticEscapedJSONFields(text)
	text = openAIPersonalAccessTokenDiagnosticJSONFieldRe.ReplaceAllString(text, `$1"[redacted]"`)
	text = redactOpenAIPersonalAccessTokenDiagnosticJSONRemainderFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticDecodedJSONFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticLayeredEscapedJSONFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticEscapedJSONFields(text)
	text = redactOpenAIPersonalAccessTokenDiagnosticKVFields(text)
	text = openAIPersonalAccessTokenDiagnosticEmailRe.ReplaceAllString(text, "[redacted]")
	text = openAIPersonalAccessTokenDiagnosticValueRe.ReplaceAllString(text, "[redacted]")
	return text
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticJSON(text string) string {
	if text == "" {
		return text
	}
	if len(text) > openAIPersonalAccessTokenDiagnosticMaxBytes {
		return sanitizeOpenAIPersonalAccessTokenDiagnosticFields(text[:openAIPersonalAccessTokenDiagnosticMaxBytes])
	}
	raw := strings.TrimSpace(text)
	if raw == "" {
		return text
	}
	if sanitized, ok := sanitizeOpenAIPersonalAccessTokenDiagnosticJSONBody([]byte(raw)); ok && strings.TrimSpace(sanitized) != "" {
		return sanitized
	}
	return text
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticJSONBody(body []byte) (string, bool) {
	if len(body) > openAIPersonalAccessTokenDiagnosticMaxBytes {
		return "", false
	}
	if !json.Valid(body) {
		return "", false
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}

	redacted := redactOpenAIPersonalAccessTokenDiagnosticJSONValue(value, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func redactOpenAIPersonalAccessTokenDiagnosticJSONValue(value any, depth int) any {
	if depth > openAISensitiveDiagnosticJSONMaxDepth {
		return "[redacted]"
	}

	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactOpenAIPersonalAccessTokenDiagnosticJSONValue(value, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = redactOpenAIPersonalAccessTokenDiagnosticJSONValue(value, depth+1)
		}
		return out
	case string:
		return sanitizeOpenAIPersonalAccessTokenDiagnosticJSONString(typed, depth)
	default:
		return value
	}
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticJSONString(value string, depth int) string {
	if depth >= openAISensitiveDiagnosticJSONMaxDepth {
		return sanitizeOpenAIPersonalAccessTokenDiagnosticFields(value)
	}

	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 0 && len(trimmed) <= openAISensitiveDiagnosticEmbeddedJSONMaxParse && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		var embedded any
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.UseNumber()
		if err := decoder.Decode(&embedded); err == nil {
			redacted := redactOpenAIPersonalAccessTokenDiagnosticJSONValue(embedded, depth+1)
			if encoded, err := json.Marshal(redacted); err == nil {
				return string(encoded)
			}
		}
	}

	return sanitizeOpenAIPersonalAccessTokenDiagnosticFields(value)
}

func redactOpenAIPersonalAccessTokenDiagnosticKVFields(text string) string {
	matches := openAIPersonalAccessTokenDiagnosticKVKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for matchIndex, match := range matches {
		if match[0] < last {
			continue
		}
		valueStart := match[1]
		b.WriteString(text[last:valueStart])
		b.WriteString("[redacted]")
		changed = true
		directValueStart := openAIPersonalAccessTokenDiagnosticDirectBearerValueStart(text, valueStart)
		valueEnd, ok := openAIPersonalAccessTokenDiagnosticDirectKVValueEnd(text, directValueStart)
		if !ok {
			return b.String()
		}
		if openAIPersonalAccessTokenDiagnosticDirectKVUnsafeNextBoundary(text, matches, matchIndex, directValueStart, valueEnd) {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func openAIPersonalAccessTokenDiagnosticDirectBearerValueStart(text string, start int) int {
	const bearer = "Bearer"
	if len(text)-start < len(bearer) || !strings.EqualFold(text[start:start+len(bearer)], bearer) {
		return start
	}
	i := start + len(bearer)
	if i >= len(text) || !openAIPersonalAccessTokenDiagnosticIsSpace(text[i]) {
		return start
	}
	for i < len(text) && openAIPersonalAccessTokenDiagnosticIsSpace(text[i]) {
		i++
	}
	return i
}

func openAIPersonalAccessTokenDiagnosticDirectKVUnsafeNextBoundary(text string, matches [][]int, matchIndex int, valueStart int, valueEnd int) bool {
	if valueStart < valueEnd && openAIPersonalAccessTokenDiagnosticDirectKVValueHasSensitiveBoundary(text[valueStart:valueEnd]) {
		return true
	}
	if matchIndex+1 >= len(matches) {
		return false
	}
	nextStart := matches[matchIndex+1][0]
	if nextStart < valueEnd {
		return true
	}
	return nextStart == valueEnd && valueEnd > 0 && openAIPersonalAccessTokenDiagnosticDirectKVClosedValueByte(text[valueEnd-1])
}

func openAIPersonalAccessTokenDiagnosticDirectKVValueHasSensitiveBoundary(value string) bool {
	return openAIPersonalAccessTokenDiagnosticJSONKeyRe.FindStringIndex(value) != nil ||
		openAIPersonalAccessTokenDiagnosticEscapedJSONKeyRe.FindStringIndex(value) != nil
}

func openAIPersonalAccessTokenDiagnosticDirectKVClosedValueByte(ch byte) bool {
	switch ch {
	case '"', '\'', '}', ']':
		return true
	default:
		return false
	}
}

func openAIPersonalAccessTokenDiagnosticDirectKVValueEnd(text string, start int) (int, bool) {
	if start >= len(text) {
		return len(text), true
	}

	switch text[start] {
	case '"':
		return openAISensitiveDiagnosticQuotedStringEnd(text, start)
	case '\'':
		return openAIPersonalAccessTokenDiagnosticSingleQuotedStringEnd(text, start)
	case '{', '[':
		return openAISensitiveDiagnosticJSONCompositeEnd(text, start)
	default:
		return openAIPersonalAccessTokenDiagnosticDirectScalarEnd(text, start), true
	}
}

func openAIPersonalAccessTokenDiagnosticSingleQuotedStringEnd(text string, start int) (int, bool) {
	escaped := false
	for i := start + 1; i < len(text); i++ {
		switch {
		case escaped:
			escaped = false
		case text[i] == '\\':
			escaped = true
		case text[i] == '\'':
			return i + 1, true
		}
	}
	return 0, false
}

func openAIPersonalAccessTokenDiagnosticDirectScalarEnd(text string, start int) int {
	for i := start; i < len(text); i++ {
		if openAIPersonalAccessTokenDiagnosticIsSpace(text[i]) || text[i] == ',' || text[i] == ';' {
			return i
		}
	}
	return len(text)
}

func openAIPersonalAccessTokenDiagnosticIsSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func redactOpenAIPersonalAccessTokenDiagnosticDecodedJSONFields(text string) string {
	text = redactOpenAIPersonalAccessTokenDiagnosticDecodedJSONKeyFields(text)
	return redactOpenAIPersonalAccessTokenDiagnosticDecodedEscapedJSONKeyFields(text)
}

func redactOpenAIPersonalAccessTokenDiagnosticDecodedJSONKeyFields(text string) string {
	var b strings.Builder
	last := 0
	changed := false
	for i := 0; i < len(text); i++ {
		if text[i] != '"' || openAISensitiveDiagnosticIsEscapedJSONQuote(text, i) {
			continue
		}

		keyEnd, ok := openAISensitiveDiagnosticQuotedStringEnd(text, i)
		if !ok {
			break
		}
		valueStart, ok := openAIPersonalAccessTokenDiagnosticJSONKeyValueStart(text, keyEnd)
		if !ok {
			i = keyEnd - 1
			continue
		}

		key, ok := openAIPersonalAccessTokenDiagnosticUnquoteJSONKey(text[i:keyEnd])
		if ok && !isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key) {
			i = keyEnd - 1
			continue
		}
		if !ok && !isOpenAIPersonalAccessTokenDiagnosticPlausiblySensitiveRawJSONKey(text[i:keyEnd]) {
			i = keyEnd - 1
			continue
		}

		b.WriteString(text[last:valueStart])
		b.WriteString(`"[redacted]"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
		i = valueEnd - 1
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func redactOpenAIPersonalAccessTokenDiagnosticDecodedEscapedJSONKeyFields(text string) string {
	var b strings.Builder
	last := 0
	changed := false
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '\\' || text[i+1] != '"' || !openAISensitiveDiagnosticIsEscapedJSONQuote(text, i+1) {
			continue
		}

		keyEnd, ok := openAISensitiveDiagnosticEscapedQuotedStringEnd(text, i)
		if !ok {
			break
		}
		valueStart, ok := openAIPersonalAccessTokenDiagnosticJSONKeyValueStart(text, keyEnd)
		if !ok {
			i = keyEnd - 1
			continue
		}

		key, ok := openAIPersonalAccessTokenDiagnosticUnquoteEscapedJSONKey(text[i:keyEnd])
		if ok && !isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key) {
			i = keyEnd - 1
			continue
		}
		if !ok && !isOpenAIPersonalAccessTokenDiagnosticPlausiblySensitiveRawJSONKey(text[i:keyEnd]) {
			i = keyEnd - 1
			continue
		}

		b.WriteString(text[last:valueStart])
		b.WriteString(`\"[redacted]\"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticEscapedJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
		i = valueEnd - 1
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func openAIPersonalAccessTokenDiagnosticJSONKeyValueStart(text string, keyEnd int) (int, bool) {
	i := keyEnd
	for i < len(text) && openAIPersonalAccessTokenDiagnosticIsSpace(text[i]) {
		i++
	}
	if i >= len(text) || text[i] != ':' {
		return 0, false
	}
	i++
	for i < len(text) && openAIPersonalAccessTokenDiagnosticIsSpace(text[i]) {
		i++
	}
	if i >= len(text) {
		return 0, false
	}
	return i, true
}

func openAIPersonalAccessTokenDiagnosticUnquoteJSONKey(quoted string) (string, bool) {
	if len(quoted) < 2 || len(quoted) > openAIPersonalAccessTokenDiagnosticMaxKeyBytes || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		return "", false
	}
	var key string
	if err := json.Unmarshal([]byte(quoted), &key); err != nil {
		return "", false
	}
	return key, true
}

func openAIPersonalAccessTokenDiagnosticUnquoteEscapedJSONKey(quoted string) (string, bool) {
	if len(quoted) < 4 || quoted[0] != '\\' || quoted[1] != '"' || quoted[len(quoted)-2] != '\\' || quoted[len(quoted)-1] != '"' {
		return "", false
	}
	if len(quoted)-2 > openAIPersonalAccessTokenDiagnosticMaxKeyBytes {
		return "", false
	}
	return openAIPersonalAccessTokenDiagnosticUnquoteJSONKey(`"` + quoted[2:len(quoted)-2] + `"`)
}

func isOpenAIPersonalAccessTokenDiagnosticPlausiblySensitiveRawJSONKey(quoted string) bool {
	raw, ok := openAIPersonalAccessTokenDiagnosticRawJSONKeyContent(quoted)
	if !ok {
		return false
	}
	raw = openAIPersonalAccessTokenDiagnosticRawJSONKeyTail(raw)
	key := openAIPersonalAccessTokenDiagnosticNormalizeRawJSONKeyTail(raw)
	if isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key) {
		return true
	}
	if strings.HasSuffix(key, "token") {
		prefix := strings.TrimSuffix(key, "token")
		if strings.Contains(prefix, "_") || strings.Contains(prefix, "-") {
			return true
		}
	}
	for _, fragment := range openAIPersonalAccessTokenDiagnosticRawSensitiveKeyFragments {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func openAIPersonalAccessTokenDiagnosticRawJSONKeyContent(quoted string) (string, bool) {
	if len(quoted) >= 2 && quoted[0] == '"' && quoted[len(quoted)-1] == '"' {
		return quoted[1 : len(quoted)-1], true
	}
	if len(quoted) >= 4 && quoted[0] == '\\' && quoted[1] == '"' && quoted[len(quoted)-2] == '\\' && quoted[len(quoted)-1] == '"' {
		return quoted[2 : len(quoted)-2], true
	}
	return "", false
}

func openAIPersonalAccessTokenDiagnosticRawJSONKeyTail(raw string) string {
	if len(raw) <= openAIPersonalAccessTokenDiagnosticRawKeyTailBytes {
		return raw
	}
	start := len(raw) - openAIPersonalAccessTokenDiagnosticRawKeyTailBytes
	for back := 1; back <= 5 && start-back >= 0; back++ {
		if raw[start-back] == '\\' {
			start -= back
			break
		}
	}
	return raw[start:]
}

func openAIPersonalAccessTokenDiagnosticNormalizeRawJSONKeyTail(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+1 < len(raw) {
			if raw[i+1] == '\\' && i+2 < len(raw) && (raw[i+2] == 'u' || raw[i+2] == 'U') {
				if ch, ok := openAIPersonalAccessTokenDiagnosticASCIIJSONHexEscape(raw, i+3); ok {
					b.WriteByte(openAIPersonalAccessTokenDiagnosticLowerASCII(ch))
					i += 7
					continue
				}
			}
			if raw[i+1] == 'u' || raw[i+1] == 'U' {
				if ch, ok := openAIPersonalAccessTokenDiagnosticASCIIJSONHexEscape(raw, i+2); ok {
					b.WriteByte(openAIPersonalAccessTokenDiagnosticLowerASCII(ch))
					i += 6
					continue
				}
			}
			b.WriteByte(openAIPersonalAccessTokenDiagnosticLowerASCII(raw[i+1]))
			i += 2
			continue
		}
		b.WriteByte(openAIPersonalAccessTokenDiagnosticLowerASCII(raw[i]))
		i++
	}
	return strings.TrimSpace(b.String())
}

func openAIPersonalAccessTokenDiagnosticASCIIJSONHexEscape(raw string, start int) (byte, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	value := 0
	for i := start; i < start+4; i++ {
		nibble, ok := openAIPersonalAccessTokenDiagnosticHexNibble(raw[i])
		if !ok {
			return 0, false
		}
		value = (value << 4) | nibble
	}
	if value > 0x7f {
		return 0, false
	}
	return byte(value), true
}

func openAIPersonalAccessTokenDiagnosticHexNibble(ch byte) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10, true
	default:
		return 0, false
	}
}

func openAIPersonalAccessTokenDiagnosticLowerASCII(ch byte) byte {
	if ch >= 'A' && ch <= 'Z' {
		return ch + ('a' - 'A')
	}
	return ch
}

func redactOpenAIPersonalAccessTokenDiagnosticJSONRemainderFields(text string) string {
	matches := openAIPersonalAccessTokenDiagnosticJSONKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, match := range matches {
		if match[0] < last {
			continue
		}
		if openAISensitiveDiagnosticIsEscapedJSONQuote(text, match[0]) {
			continue
		}
		valueStart := match[1]
		b.WriteString(text[last:valueStart])
		b.WriteString(`"[redacted]"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func redactOpenAIPersonalAccessTokenDiagnosticEscapedJSONFields(text string) string {
	matches := openAIPersonalAccessTokenDiagnosticEscapedJSONKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, match := range matches {
		if match[0] < last {
			continue
		}
		valueStart := match[1]
		b.WriteString(text[last:valueStart])
		b.WriteString(`\"[redacted]\"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticEscapedJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func redactOpenAIPersonalAccessTokenDiagnosticLayeredEscapedJSONFields(text string) string {
	var b strings.Builder
	last := 0
	changed := false
	for quoteIndex := 0; quoteIndex < len(text); quoteIndex++ {
		if text[quoteIndex] != '"' {
			continue
		}
		keyQuoteStart, ok := openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenStart(text, quoteIndex)
		if !ok || keyQuoteStart < last {
			continue
		}

		keyEndTokenStart, keyEnd, ok := openAIPersonalAccessTokenDiagnosticNextEscapedQuoteToken(text, quoteIndex+1)
		if !ok {
			break
		}
		valueStart, ok := openAIPersonalAccessTokenDiagnosticJSONKeyValueStart(text, keyEnd)
		if !ok {
			quoteIndex = keyEnd - 1
			continue
		}

		rawKey := text[quoteIndex+1 : keyEndTokenStart]
		if !isOpenAIPersonalAccessTokenDiagnosticSensitiveLayeredEscapedJSONKey(rawKey) {
			quoteIndex = keyEnd - 1
			continue
		}

		b.WriteString(text[last:valueStart])
		replacement := openAIPersonalAccessTokenDiagnosticLayeredEscapedRedactedValue(text, valueStart)
		b.WriteString(replacement)
		changed = true
		valueEnd, ok := openAIPersonalAccessTokenDiagnosticLayeredEscapedJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
		quoteIndex = valueEnd - 1
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func isOpenAIPersonalAccessTokenDiagnosticSensitiveLayeredEscapedJSONKey(rawKey string) bool {
	if len(rawKey) <= openAIPersonalAccessTokenDiagnosticMaxKeyBytes {
		if key, ok := openAIPersonalAccessTokenDiagnosticUnquoteJSONKey(`"` + rawKey + `"`); ok {
			return isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key)
		}
	}
	return isOpenAIPersonalAccessTokenDiagnosticPlausiblySensitiveRawJSONKey(`"` + rawKey + `"`)
}

func openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenStart(text string, quoteIndex int) (int, bool) {
	if quoteIndex <= 0 || quoteIndex >= len(text) || text[quoteIndex] != '"' {
		return 0, false
	}
	start := quoteIndex - 1
	for start >= 0 && text[start] == '\\' {
		start--
	}
	start++
	if start == quoteIndex {
		return 0, false
	}
	if (quoteIndex-start)%2 == 0 {
		return 0, false
	}
	return start, true
}

func openAIPersonalAccessTokenDiagnosticNextEscapedQuoteToken(text string, start int) (int, int, bool) {
	for i := start; i < len(text); i++ {
		if text[i] != '"' {
			continue
		}
		tokenStart, ok := openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenStart(text, i)
		if ok {
			return tokenStart, i + 1, true
		}
	}
	return 0, 0, false
}

func openAIPersonalAccessTokenDiagnosticLayeredEscapedJSONValueEnd(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}
	if quoteIndex, ok := openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenAt(text, start); ok {
		for next := quoteIndex + 1; next < len(text); next++ {
			if text[next] != '"' {
				continue
			}
			_, valueEnd, ok := openAIPersonalAccessTokenDiagnosticNextEscapedQuoteToken(text, next)
			if !ok {
				return 0, false
			}
			if openAISensitiveDiagnosticValueHasJSONDelimiter(text, valueEnd) {
				return valueEnd, true
			}
			next = valueEnd - 1
		}
		return 0, false
	}
	switch text[start] {
	case '\\':
		return 0, false
	case '{', '[':
		return openAISensitiveDiagnosticJSONCompositeEnd(text, start)
	default:
		valueEnd, ok := openAISensitiveDiagnosticUnknownScalarEnd(text, start)
		if !ok || !openAISensitiveDiagnosticValueHasJSONDelimiter(text, valueEnd) {
			return 0, false
		}
		return valueEnd, true
	}
}

func openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenAt(text string, start int) (int, bool) {
	if start >= len(text) || text[start] != '\\' {
		return 0, false
	}
	i := start
	for i < len(text) && text[i] == '\\' {
		i++
	}
	if i >= len(text) || text[i] != '"' || (i-start)%2 == 0 {
		return 0, false
	}
	return i, true
}

func openAIPersonalAccessTokenDiagnosticLayeredEscapedRedactedValue(text string, start int) string {
	quoteIndex, ok := openAIPersonalAccessTokenDiagnosticEscapedQuoteTokenAt(text, start)
	if !ok {
		return `"[redacted]"`
	}
	return text[start:quoteIndex+1] + `[redacted]` + text[start:quoteIndex+1]
}

func isOpenAIPersonalAccessTokenDiagnosticSensitiveJSONKey(key string) bool {
	key = openAIPersonalAccessTokenDiagnosticNormalizeDecodedJSONKey(key)
	if key == "" {
		return false
	}
	if isOpenAISensitiveDiagnosticField(key) {
		return true
	}
	return strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "-token") || strings.HasSuffix(key, "token")
}

func openAIPersonalAccessTokenDiagnosticNormalizeDecodedJSONKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) > openAIPersonalAccessTokenDiagnosticRawKeyTailBytes {
		key = openAIPersonalAccessTokenDiagnosticRawJSONKeyTail(key)
	}
	return openAIPersonalAccessTokenDiagnosticNormalizeRawJSONKeyTail(key)
}

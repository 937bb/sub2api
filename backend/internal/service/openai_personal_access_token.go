package service

import (
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
	openAIPersonalAccessTokenPrefix = "at-"
	defaultOpenAIAuthAPIBaseURL     = "https://auth.openai.com/api/accounts"
	openAIAuthAPIBaseURLEnvVar      = "CODEX_AUTHAPI_BASE_URL"
	openAIWhoamiPath                = "/v1/user-auth-credential/whoami"
)

var openAIAuthAPIBaseURL = defaultOpenAIAuthAPIBaseURL

var (
	openAIPersonalAccessTokenDiagnosticFieldPattern = `personal_access_token|access_token|refresh_token|id_token|session_token|authorization|api_key|apikey|token|email|chatgpt_user_id|chatgpt_account_id|chatgpt_plan_type|chatgpt_account_is_fedramp`
	openAIPersonalAccessTokenDiagnosticJSONFieldRe  = regexp.MustCompile(`(?i)("(?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)"\s*:\s*)(?:"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)`)
	openAIPersonalAccessTokenDiagnosticKVFieldRe    = regexp.MustCompile(`(?i)\b((?:` + openAIPersonalAccessTokenDiagnosticFieldPattern + `)\s*(?:=|:)\s*)(?:Bearer\s+)?(?:"(?:\\.|[^"\\])*"|[^\s,;"}]+)`)
	openAIPersonalAccessTokenDiagnosticEmailRe      = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	openAIPersonalAccessTokenDiagnosticValueRe      = regexp.MustCompile(`\bat-[A-Za-z0-9._~+/=-]+`)
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
	text = sanitizeOpenAIUpstreamDiagnosticText(text)
	text = openAIPersonalAccessTokenDiagnosticJSONFieldRe.ReplaceAllString(text, `$1"[redacted]"`)
	text = openAIPersonalAccessTokenDiagnosticKVFieldRe.ReplaceAllString(text, `$1[redacted]`)
	text = openAIPersonalAccessTokenDiagnosticEmailRe.ReplaceAllString(text, "[redacted]")
	text = openAIPersonalAccessTokenDiagnosticValueRe.ReplaceAllString(text, "[redacted]")
	return text
}

func sanitizeOpenAIPersonalAccessTokenDiagnosticJSON(text string) string {
	raw := []byte(strings.TrimSpace(text))
	if len(raw) == 0 || !json.Valid(raw) {
		return text
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return text
	}
	encoded, err := json.Marshal(redactOpenAIPersonalAccessTokenDiagnosticJSON(decoded))
	if err != nil {
		return text
	}
	return string(encoded)
}

func redactOpenAIPersonalAccessTokenDiagnosticJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if isOpenAIPersonalAccessTokenDiagnosticSensitiveKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactOpenAIPersonalAccessTokenDiagnosticJSON(value)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, value := range typed {
			out = append(out, redactOpenAIPersonalAccessTokenDiagnosticJSON(value))
		}
		return out
	default:
		return value
	}
}

func isOpenAIPersonalAccessTokenDiagnosticSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "email",
		"chatgpt_user_id",
		"chatgpt_account_id",
		"chatgpt_plan_type",
		"chatgpt_account_is_fedramp",
		"personal_access_token",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"authorization",
		"api_key",
		"apikey",
		"token":
		return true
	}
	return strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "-token")
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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

// OpenAIPersonalAccessTokenMetadata is the official Codex whoami response for
// personal access tokens.
type OpenAIPersonalAccessTokenMetadata struct {
	Email                   string `json:"email"`
	ChatGPTUserID           string `json:"chatgpt_user_id"`
	ChatGPTAccountID        string `json:"chatgpt_account_id"`
	ChatGPTPlanType         string `json:"chatgpt_plan_type"`
	ChatGPTAccountIsFedRAMP bool   `json:"chatgpt_account_is_fedramp"`
}

type openAIPersonalAccessTokenWhoamiError struct {
	statusCode int
	body       string
}

func (e *openAIPersonalAccessTokenWhoamiError) Error() string {
	if e == nil {
		return "OpenAI PAT whoami failed"
	}
	body := sanitizeOpenAIUpstreamDiagnosticText(truncateOpenAIWhoamiBody(e.body))
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

// OpenAIPersonalAccessTokenFingerprint returns a non-secret stable fingerprint
// used to detect when persisted whoami metadata belongs to a different PAT.
func OpenAIPersonalAccessTokenFingerprint(personalAccessToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(personalAccessToken)))
	return hex.EncodeToString(sum[:])
}

// AccountNeedsOpenAIPersonalAccessTokenMetadataHydration reports whether the
// account is missing the official whoami metadata Codex sends alongside PATs.
func AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account *Account, personalAccessToken string) bool {
	if account == nil || account.Credentials == nil {
		return true
	}
	fingerprint := OpenAIPersonalAccessTokenFingerprint(personalAccessToken)
	storedFingerprint := strings.TrimSpace(account.GetCredential("personal_access_token_sha256"))
	if storedFingerprint == "" || !strings.EqualFold(storedFingerprint, fingerprint) {
		return true
	}
	for _, key := range []string{
		"email",
		"chatgpt_user_id",
		"chatgpt_account_id",
		"chatgpt_plan_type",
	} {
		if strings.TrimSpace(account.GetCredential(key)) == "" {
			return true
		}
	}
	if _, ok := account.Credentials["chatgpt_account_is_fedramp"]; !ok {
		return true
	}
	return false
}

// BuildOpenAIPersonalAccessTokenCredentialUpdates converts whoami metadata into
// account credential fields used by the Codex request path.
func BuildOpenAIPersonalAccessTokenCredentialUpdates(personalAccessToken string, metadata *OpenAIPersonalAccessTokenMetadata) map[string]any {
	updates := map[string]any{
		"personal_access_token_sha256":      OpenAIPersonalAccessTokenFingerprint(personalAccessToken),
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
	_, err := repo.BulkUpdate(ctx, []int64{account.ID}, AccountBulkUpdate{Credentials: updates})
	return err
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

	var metadata OpenAIPersonalAccessTokenMetadata
	resp, err := client.R().
		SetContext(reqCtx).
		SetHeader("Authorization", "Bearer "+personalAccessToken).
		SetSuccessResult(&metadata).
		Get(openAIWhoamiURL())
	if err != nil {
		return nil, fmt.Errorf("OpenAI PAT whoami request failed: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("OpenAI PAT whoami request returned no response")
	}
	if !resp.IsSuccessState() {
		return nil, &openAIPersonalAccessTokenWhoamiError{statusCode: resp.StatusCode, body: resp.String()}
	}
	if err := metadata.validate(); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (m *OpenAIPersonalAccessTokenMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("OpenAI PAT whoami returned empty metadata")
	}
	missing := make([]string, 0, 4)
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
	if len(missing) > 0 {
		return fmt.Errorf("OpenAI PAT whoami metadata missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
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

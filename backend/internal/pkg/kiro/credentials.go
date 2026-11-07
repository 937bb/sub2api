// Package kiro implements the Kiro IDE protocol documented by hank9999/kiro.rs.
package kiro

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const Version = "0.11.107"

var regionPattern = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]+$`)
var machinePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)

// Normalize imports kiro.rs JSON names into the gateway's credential convention.
// Unknown non-authentication fields are retained for model mapping compatibility.
func Normalize(input map[string]any) (map[string]any, error) {
	out := WithoutNullCredentialFields(input)
	if raw, exists := out["credentials"]; exists {
		nested, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("Kiro credentials must be an object")
		}
		delete(out, "credentials")
		for k, v := range nested {
			out[k] = v
		}
	}
	for _, key := range []string{"expiresAt", "expires_at"} {
		if n, ok := out[key].(float64); ok {
			if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 32503680000000 {
				return nil, errors.New("invalid Kiro expiry timestamp")
			}
			if n > 1e11 {
				n /= 1000
			}
			out[key] = time.Unix(int64(n), 0).UTC().Format(time.RFC3339)
		}
	}
	for _, key := range []string{"csrfToken", "proxy_id", "proxyId", "proxy", "traffic", "subscription", "usage", "tags", "status", "createdAt", "lastUsedAt", "id", "disabled", "password"} {
		delete(out, key)
	}
	aliases := map[string]string{"accessToken": "access_token", "refreshToken": "refresh_token", "expiresAt": "expires_at", "clientId": "client_id", "clientSecret": "client_secret", "profileArn": "profile_arn", "authMethod": "auth_method", "authRegion": "auth_region", "apiRegion": "api_region", "machineId": "machine_id", "kiroApiKey": "api_key", "kiroVersion": "kiro_version", "systemVersion": "system_version", "nodeVersion": "node_version", "idToken": "id_token"}
	for from, to := range aliases {
		if v, ok := out[from]; ok {
			if old, exists := out[to]; exists && !reflect.DeepEqual(old, v) {
				return nil, fmt.Errorf("conflicting Kiro field %s", to)
			}
			out[to] = v
			delete(out, from)
		}
	}
	for _, key := range []string{"access_token", "id_token", "refresh_token", "client_id", "client_secret", "profile_arn", "auth_method", "auth_region", "api_region", "region", "machine_id", "expires_at", "api_key", "endpoint"} {
		if v, ok := out[key]; ok {
			s, valid := v.(string)
			if !valid {
				return nil, fmt.Errorf("Kiro %s must be a string", key)
			}
			s = strings.TrimSpace(s)
			if strings.ContainsAny(s, "\r\n\x00") {
				return nil, fmt.Errorf("invalid Kiro %s", key)
			}
			out[key] = s
		}
	}
	method := strings.ToLower(String(out, "auth_method"))
	if method == "" {
		method = "social"
		if String(out, "client_id") != "" {
			method = "idc"
		}
		if String(out, "api_key") != "" {
			method = "api_key"
		}
	}
	switch method {
	case "builder-id", "iam":
		method = "idc"
	case "apikey":
		method = "api_key"
	}
	if method != "social" && method != "idc" && method != "api_key" {
		return nil, errors.New("Kiro authMethod must be social, idc or api_key")
	}
	out["auth_method"] = method
	if method == "api_key" {
		if String(out, "api_key") == "" {
			return nil, errors.New("Kiro kiroApiKey is required")
		}
	} else {
		if String(out, "refresh_token") == "" {
			return nil, errors.New("Kiro refreshToken is required")
		}
		if method == "idc" && (String(out, "client_id") == "" || String(out, "client_secret") == "") {
			return nil, errors.New("Kiro IdC requires clientId and clientSecret")
		}
	}
	for _, key := range []string{"region", "auth_region", "api_region"} {
		if s := String(out, key); s != "" && !regionPattern.MatchString(s) {
			return nil, fmt.Errorf("invalid Kiro %s", key)
		}
	}
	if s := String(out, "expires_at"); s != "" {
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return nil, errors.New("Kiro expiresAt must be RFC3339")
		}
	}
	if String(out, "machine_id") == "" {
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		out["machine_id"] = hex.EncodeToString(b[:])
	}
	if !machinePattern.MatchString(String(out, "machine_id")) {
		return nil, errors.New("invalid Kiro machineId")
	}
	if ep := String(out, "endpoint"); ep != "" && ep != "ide" && ep != "runtime_v3" && ep != "cli" {
		return nil, errors.New("Kiro endpoint must be runtime_v3, ide or cli")
	}
	if err := normalizeEnvironment(out); err != nil {
		return nil, err
	}
	return out, nil
}
func String(c map[string]any, k string) string { s, _ := c[k].(string); return s }

// WithoutNullCredentialFields treats nullable export fields as absent, including
// during credential updates. Explicit non-null values still undergo validation.
// Both maps are copied so caller-owned account snapshots remain immutable.
func WithoutNullCredentialFields(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		if v == nil {
			switch k {
			case "credentials", "access_token", "accessToken", "refresh_token", "refreshToken",
				"id_token", "idToken", "client_id", "clientId", "client_secret", "clientSecret",
				"expires_at", "expiresAt", "profile_arn", "profileArn", "auth_method", "authMethod",
				"auth_region", "authRegion", "api_region", "apiRegion", "region",
				"machine_id", "machineId", "api_key", "kiroApiKey", "endpoint",
				"kiro_version", "kiroVersion", "system_version", "systemVersion", "node_version", "nodeVersion":
				continue
			}
		}
		if k == "credentials" {
			if nested, ok := v.(map[string]any); ok {
				v = WithoutNullCredentialFields(nested)
			}
		}
		out[k] = v
	}
	return out
}

func Region(c map[string]any, kind string) string {
	if s := String(c, kind+"_region"); s != "" {
		return s
	}
	// A real profile belongs to a data-plane region; it does not change the
	// authentication region. Explicit api_region remains authoritative.
	if kind == "api" {
		parts := strings.Split(String(c, "profile_arn"), ":")
		if len(parts) == 6 && parts[0] == "arn" && parts[1] == "aws" &&
			parts[2] == "codewhisperer" && regionPattern.MatchString(parts[3]) &&
			len(parts[4]) == 12 && strings.Trim(parts[4], "0123456789") == "" &&
			strings.HasPrefix(parts[5], "profile/") && len(parts[5]) > len("profile/") &&
			String(c, "profile_arn") != "arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX" {
			return parts[3]
		}
	}
	if s := String(c, "region"); s != "" {
		return s
	}
	return "us-east-1"
}
func Endpoint(c map[string]any) string {
	if EndpointMode(c) == "cli" {
		return "https://q." + Region(c, "api") + ".amazonaws.com/"
	}
	return "https://runtime." + Region(c, "api") + ".kiro.dev/"
}
func RefreshPayload(c map[string]any) (string, map[string]any) {
	if String(c, "auth_method") == "idc" {
		return "https://oidc." + Region(c, "auth") + ".amazonaws.com/token", map[string]any{"clientId": String(c, "client_id"), "clientSecret": String(c, "client_secret"), "refreshToken": String(c, "refresh_token"), "grantType": "refresh_token"}
	}
	return "https://prod." + Region(c, "auth") + ".auth.desktop.kiro.dev/refreshToken", map[string]any{"refreshToken": String(c, "refresh_token")}
}
func Headers(h http.Header, c map[string]any, token string) {
	h.Set("Content-Type", "application/x-amz-json-1.0")
	h.Set("X-Amz-Target", "KiroRuntimeService.GenerateAssistantResponse")
	if EndpointMode(c) == "cli" {
		h.Set("X-Amz-Target", "AmazonCodeWhispererStreamingService.GenerateAssistantResponse")
	}
	h.Set("Accept", "application/vnd.amazon.eventstream")
	h.Set("Authorization", "Bearer "+token)
	h.Set("x-amzn-codewhisperer-optout", "true")
	h.Set("x-amzn-kiro-agent-mode", "vibe")
	if EndpointMode(c) == "cli" {
		CLIHeaders(h, c)
	} else {
		EnvironmentHeaders(h, c, "streaming")
	}
	if String(c, "auth_method") == "api_key" {
		h.Set("tokentype", "API_KEY")
	}
}

// ImportedAccount accepts both kiro.rs and Kiro Account Manager exports.
type ImportedAccount struct {
	Name        string
	Credentials map[string]any
	Disabled    bool
}

func ParseImport(data []byte) ([]ImportedAccount, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, errors.New("invalid Kiro credential JSON")
	}
	if obj, ok := root.(map[string]any); ok {
		if accounts, exists := obj["accounts"]; exists {
			root = accounts
		} else {
			root = []any{obj}
		}
	}
	rows, ok := root.([]any)
	if !ok || len(rows) == 0 || len(rows) > 500 {
		return nil, errors.New("Kiro import requires 1..500 accounts")
	}
	out := make([]ImportedAccount, 0, len(rows))
	for i, row := range rows {
		obj, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Kiro account %d must be an object", i+1)
		}
		creds, err := Normalize(obj)
		if err != nil {
			return nil, fmt.Errorf("Kiro account %d: %w", i+1, err)
		}
		name := String(obj, "email")
		if name == "" {
			name = String(obj, "name")
		}
		if name == "" {
			name = fmt.Sprintf("Kiro %d", i+1)
		}
		disabled, _ := obj["disabled"].(bool)
		out = append(out, ImportedAccount{Name: name, Credentials: creds, Disabled: disabled})
	}
	return out, nil
}

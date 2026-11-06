package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

const (
	StreamingSDKVersion = "1.0.0"
	RuntimeSDKVersion   = "1.0.0"
	SSOSDKVersion       = "3.980.0"
)

type Operation string

const (
	OperationGeneration Operation = "generation"
	OperationModels     Operation = "models"
	OperationUsage      Operation = "usage"
	OperationRefresh    Operation = "refresh"
)

// Environment is a credential-free view of the effective account identity.
// The transport is Go TLS, not a measured replica of the Kiro desktop handshake.
type Environment struct {
	MachineID     string `json:"machine_id"`
	KiroVersion   string `json:"kiro_version"`
	NodeVersion   string `json:"node_version"`
	SystemVersion string `json:"system_version"`
	AuthRegion    string `json:"auth_region"`
	APIRegion     string `json:"api_region"`
	AuthMethod    string `json:"auth_method"`
	Endpoint      string `json:"endpoint"`
	StreamingSDK  string `json:"streaming_sdk"`
	RuntimeSDK    string `json:"runtime_sdk"`
	SSOSDK        string `json:"sso_sdk"`
}

func AccountEnvironment(c map[string]any) Environment {
	method := String(c, "auth_method")
	if method == "" {
		method = "social"
	}
	return Environment{
		MachineID: String(c, "machine_id"), KiroVersion: environment(c, "kiro_version"),
		NodeVersion: environment(c, "node_version"), SystemVersion: environment(c, "system_version"),
		AuthRegion: Region(c, "auth"), APIRegion: Region(c, "api"), AuthMethod: method,
		Endpoint: "runtime_v3", StreamingSDK: StreamingSDKVersion, RuntimeSDK: RuntimeSDKVersion, SSOSDK: SSOSDKVersion,
	}
}

// EnvironmentKey excludes rotating tokens and request IDs. It invalidates
// identity-dependent caches when the account's environment changes.
func EnvironmentKey(c map[string]any) string {
	b, _ := json.Marshal(AccountEnvironment(c))
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func ModelsURL(c map[string]any) string {
	return "https://management." + Region(c, "api") + ".kiro.dev/"
}

// OperationHeaders is shared by real requests and redacted admin diagnostics.
func OperationHeaders(h http.Header, c map[string]any, token string, operation Operation) {
	// A reused header map must not carry credentials or operation selectors from
	// a preceding request (particularly refresh requests).
	for _, key := range []string{"Authorization", "tokentype", "X-Amz-Target", "x-amz-user-agent", "x-amzn-codewhisperer-optout", "x-amzn-kiro-agent-mode", "amz-sdk-invocation-id", "amz-sdk-request", "Content-Type", "Accept"} {
		h.Del(key)
	}
	switch operation {
	case OperationRefresh:
		RefreshHeaders(h, c)
		return
	case OperationGeneration:
		Headers(h, c, token)
		return
	case OperationModels, OperationUsage:
		EnvironmentHeaders(h, c, "runtime")
		h.Set("Accept", "application/json")
		h.Set("Authorization", "Bearer "+token)
		if String(c, "auth_method") == "api_key" {
			h.Set("tokentype", "API_KEY")
		}
		if operation == OperationModels {
			h.Set("Content-Type", "application/x-amz-json-1.0")
			h.Set("X-Amz-Target", "AmazonCodeWhispererService.ListAvailableModels")
		}
	}
}

// PreserveEnvironment keeps one account identity when a token-only JSON import
// replaces credentials. Explicit snake/camel/nested values always take priority.
func PreserveEnvironment(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming)+8)
	for k, v := range incoming {
		out[k] = v
	}
	nested, _ := incoming["credentials"].(map[string]any)
	fields := map[string]string{"machine_id": "machineId", "kiro_version": "kiroVersion", "node_version": "nodeVersion", "system_version": "systemVersion", "auth_region": "authRegion", "api_region": "apiRegion", "region": "region", "endpoint": "endpoint", "profile_arn": "profileArn", "client_id": "clientId", "auth_method": "authMethod"}
	for canonical, alias := range fields {
		// Explicit authentication selectors let Normalize infer a new method.
		// A token-only update keeps the existing IdC/social/API-key identity.
		if canonical == "auth_method" {
			explicitSelector := false
			for _, key := range []string{"api_key", "kiroApiKey", "client_id", "clientId"} {
				_, top := incoming[key]
				_, inner := nested[key]
				explicitSelector = explicitSelector || top || inner
			}
			if explicitSelector {
				continue
			}
		}
		_, a := incoming[canonical]
		_, b := incoming[alias]
		_, c := nested[canonical]
		_, d := nested[alias]
		if a || b || c || d {
			continue
		}
		if value, ok := existing[canonical]; ok {
			out[canonical] = value
		}
	}
	return out
}

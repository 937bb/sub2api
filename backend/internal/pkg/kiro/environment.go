package kiro

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

var environmentPatterns = map[string]*regexp.Regexp{
	"kiro_version":   regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`),
	"node_version":   regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`),
	"system_version": regexp.MustCompile(`^(darwin|win32|linux)#[A-Za-z0-9._-]+$`),
}
var environmentDefaults = map[string]string{
	"kiro_version": Version, "node_version": "22.22.0", "system_version": "darwin#24.6.0",
}

func normalizeEnvironment(c map[string]any) error {
	for key, fallback := range environmentDefaults {
		if _, exists := c[key]; !exists {
			c[key] = fallback
		}
		value, ok := c[key].(string)
		if !ok || len(value) > 80 || !environmentPatterns[key].MatchString(value) {
			return fmt.Errorf("invalid Kiro %s", key)
		}
	}
	return nil
}

func environment(c map[string]any, key string) string {
	if value := String(c, key); len(value) <= 80 && environmentPatterns[key].MatchString(value) {
		return value
	}
	return environmentDefaults[key]
}

// EnvironmentHeaders shares stable account identity across AWS data-plane calls.
// The invocation ID identifies an individual request, never an account.
func EnvironmentHeaders(h http.Header, c map[string]any, operation string) {
	sdk, api, metrics := StreamingSDKVersion, "kiroruntime", "N"
	if operation == "runtime" {
		sdk, api, metrics = RuntimeSDKVersion, "codewhispererruntime", "N,E"
	}
	identity := "KiroIDE-" + environment(c, "kiro_version") + "-" + String(c, "machine_id")
	h.Set("x-amz-user-agent", "aws-sdk-js/"+sdk+" "+identity)
	h.Set("User-Agent", fmt.Sprintf("aws-sdk-js/%s ua/2.1 os/%s lang/js md/nodejs#%s api/%s#%s m/%s %s", sdk, environment(c, "system_version"), environment(c, "node_version"), api, sdk, metrics, identity))
	h.Set("amz-sdk-invocation-id", uuid.NewString())
	// This transport makes one attempt; do not advertise fictitious SDK retries.
	h.Set("amz-sdk-request", "attempt=1; max=1")
}

func RefreshHeaders(h http.Header, c map[string]any) {
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json, text/plain, */*")
	if String(c, "auth_method") == "idc" {
		h.Set("x-amz-user-agent", "aws-sdk-js/"+SSOSDKVersion+" KiroIDE")
		h.Set("User-Agent", fmt.Sprintf("aws-sdk-js/%s ua/2.1 os/%s lang/js md/nodejs#%s api/sso-oidc#%s m/E KiroIDE", SSOSDKVersion, environment(c, "system_version"), environment(c, "node_version"), SSOSDKVersion))
		h.Set("amz-sdk-invocation-id", uuid.NewString())
		h.Set("amz-sdk-request", "attempt=1; max=1")
	} else {
		h.Set("User-Agent", "KiroIDE-"+environment(c, "kiro_version")+"-"+String(c, "machine_id"))
	}
	// Go negotiates and decodes gzip itself. Do not advertise unsupported codecs.
}

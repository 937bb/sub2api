package service

import (
	"net/http"
	"net/url"
	"runtime"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

type KiroOperationFingerprint struct {
	Operation kiro.Operation    `json:"operation"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
}

type KiroFingerprint struct {
	Environment        kiro.Environment           `json:"environment"`
	EnvironmentKey     string                     `json:"environment_key"`
	Operations         []KiroOperationFingerprint `json:"operations"`
	ProxyID            *int64                     `json:"proxy_id"`
	ProxyReady         bool                       `json:"proxy_ready"`
	HTTPProtocol       string                     `json:"http_protocol"`
	TLSBackend         string                     `json:"tls_backend"`
	GoVersion          string                     `json:"go_version"`
	TLSCaptureVerified bool                       `json:"tls_capture_verified"`
	WebSocketSupported bool                       `json:"websocket_supported"`
}

func kiroAccountFingerprintSummary(a *Account) *AccountFingerprint {
	c := a.Credentials
	environment := kiro.AccountEnvironment(c)
	_, proxyErr := kiroProxy(a)
	refreshURL, _ := kiro.RefreshPayload(c)
	usageEndpoint := ""
	if usageURL, err := url.Parse(kiro.UsageURL(c)); err == nil {
		usageURL.RawQuery = "" // Profile ARN is not part of this diagnostic allowlist.
		usageEndpoint = usageURL.String()
	}
	ops := []KiroOperationFingerprint{
		{Operation: kiro.OperationGeneration, Method: http.MethodPost, URL: kiro.Endpoint(c)},
		{Operation: kiro.OperationModels, Method: http.MethodPost, URL: kiro.ModelsURL(c)},
		{Operation: kiro.OperationUsage, Method: http.MethodGet, URL: usageEndpoint},
	}
	if a.GetCredential("auth_method") != "api_key" {
		ops = append(ops, KiroOperationFingerprint{Operation: kiro.OperationRefresh, Method: http.MethodPost, URL: refreshURL})
	}
	for i := range ops {
		h := http.Header{}
		kiro.OperationHeaders(h, c, "", ops[i].Operation)
		ops[i].Headers = map[string]string{}
		for _, key := range []string{"User-Agent", "x-amz-user-agent", "Content-Type", "Accept", "X-Amz-Target", "tokentype", "x-amzn-codewhisperer-optout", "x-amzn-kiro-agent-mode", "amz-sdk-request"} {
			if value := h.Get(key); value != "" {
				ops[i].Headers[key] = value
			}
		}
		ops[i].Headers["Connection"] = "close"
	}
	return &AccountFingerprint{
		AccountID: a.ID, CredentialAccountID: a.ID, InstallationID: environment.MachineID,
		Source: "account_metadata", ConvergenceMode: "off", ConvergenceReady: environment.MachineID != "" && proxyErr == nil,
		WSAccountMode: "unsupported",
		Kiro: &KiroFingerprint{
			Environment: environment, EnvironmentKey: kiro.EnvironmentKey(c), Operations: ops,
			ProxyID: a.ProxyID, ProxyReady: proxyErr == nil, HTTPProtocol: "HTTP/1.1", TLSBackend: "Go crypto/tls", GoVersion: runtime.Version(),
		},
	}
}

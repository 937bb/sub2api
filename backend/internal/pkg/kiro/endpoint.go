package kiro

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
)

const (
	CLISDKVersion       = "1.3.15"
	CLIStreamingVersion = "0.1.14474"
	CLIRustVersion      = "1.92.0"
)

// EndpointMode preserves the deployed runtime-v3 route unless explicitly changed.
// The legacy "ide" selector remains an alias for that established route.
func EndpointMode(c map[string]any) string {
	if String(c, "endpoint") == "cli" {
		return "cli"
	}
	return "runtime_v3"
}

func GenerationSDKVersion(c map[string]any) string {
	if EndpointMode(c) == "cli" {
		return CLISDKVersion
	}
	return StreamingSDKVersion
}

func CLIHeaders(h http.Header, c map[string]any) {
	h.Set("User-Agent", fmt.Sprintf("aws-sdk-rust/%s ua/2.1 api/codewhispererstreaming/%s os/%s lang/rust/%s md/appVersion-%s app/AmazonQ-For-CLI", CLISDKVersion, CLIStreamingVersion, environment(c, "system_version"), CLIRustVersion, environment(c, "kiro_version")))
	h.Set("x-amz-user-agent", fmt.Sprintf("aws-sdk-rust/%s ua/2.1 api/codewhispererstreaming/%s os/%s lang/rust/%s m/F app/AmazonQ-For-CLI", CLISDKVersion, CLIStreamingVersion, environment(c, "system_version"), CLIRustVersion))
	h.Set("amz-sdk-invocation-id", uuid.NewString())
	h.Set("amz-sdk-request", "attempt=1; max=1")
}

// applyEndpointBody adjusts only Kiro transport envelope fields. It never rewrites
// user text, tool schemas, tool input/results, images, or model identities.
func applyEndpointBody(body map[string]any, c map[string]any) {
	if EndpointMode(c) != "cli" {
		return
	}
	state, ok := body["conversationState"].(map[string]any)
	if !ok {
		return
	}
	delete(state, "agentContinuationId")
	adjust := func(message any, historical bool) {
		wrapper, ok := message.(map[string]any)
		if !ok {
			return
		}
		user, ok := wrapper["userInputMessage"].(map[string]any)
		if !ok {
			return
		}
		user["origin"] = "KIRO_CLI"
		if historical {
			delete(user, "modelId")
		}
	}
	adjust(state["currentMessage"], false)
	if history, ok := state["history"].([]any); ok {
		for _, message := range history {
			adjust(message, true)
		}
	}
}

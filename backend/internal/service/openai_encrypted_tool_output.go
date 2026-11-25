package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

var errOpenAIInvalidEncryptedToolOutput = errors.New("openai request contains invalid encrypted function output")

// Tool output ciphertext is opaque, required context. Unlike optional reasoning
// recovery, rejecting it must never trigger context stripping or replay.
func isOpenAIInvalidEncryptedToolOutputMessage(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "encrypted function output") &&
		(strings.Contains(message, "could not be decrypted") || strings.Contains(message, "could not be decoded"))
}

func openAIEncryptedRequestResponseError(status int, body []byte) error {
	if status != http.StatusBadRequest {
		return nil
	}
	if isOpenAIInvalidEncryptedToolOutputMessage(extractUpstreamErrorMessage(body)) {
		return errOpenAIInvalidEncryptedToolOutput
	}
	if isOpenAIInvalidEncryptedContentResponse(status, body) {
		return errOpenAIInvalidEncryptedContent
	}
	return nil
}

func isOpenAIEncryptedRequestError(err error) bool {
	return errors.Is(err, errOpenAIInvalidEncryptedToolOutput) || isOpenAIInvalidEncryptedContentError(err) ||
		(err != nil && isOpenAIInvalidEncryptedToolOutputMessage(err.Error()))
}

// Inspect only the Responses protocol's content array, never strings containing
// JSON or arbitrary tool-owned objects that happen to use similar field names.
func openAIRequestHasEncryptedToolOutput(body []byte) bool {
	found := false
	openAIRequestPayloadView(body).Get("input").ForEach(func(_, item gjson.Result) bool {
		switch item.Get("type").String() {
		case "function_call_output", "custom_tool_call_output":
			output := item.Get("output")
			if output.IsArray() {
				output.ForEach(func(_, part gjson.Result) bool {
					encrypted := part.Get("encrypted_content")
					if part.Get("type").String() == "encrypted_content" && encrypted.Type == gjson.String && encrypted.Str != "" {
						found = true
					}
					return !found
				})
			}
		}
		return !found
	})
	return found
}

func openAIEncryptedToolOutputOwner(ctx context.Context) int64 {
	affinity, _ := ctx.Value(openAIEncryptedAffinityKey{}).(*openAIEncryptedAffinity)
	if affinity == nil || !affinity.toolOutput {
		return 0
	}
	return affinity.owner.Load()
}

func bindOpenAIEncryptedToolOutputAttempt(ctx context.Context, account *Account) {
	affinity, _ := ctx.Value(openAIEncryptedAffinityKey{}).(*openAIEncryptedAffinity)
	if affinity != nil && affinity.toolOutput && account != nil {
		affinity.owner.CompareAndSwap(0, account.ID)
	}
}

// Log only bounded ciphertext fingerprints and structural counts, never tool
// output text, ciphertext, names, headers, credentials, or the request body.
func logOpenAIEncryptedToolOutputRejection(ctx context.Context, account *Account, body []byte, err error) {
	if account == nil || !isOpenAIEncryptedRequestError(err) || !openAIRequestHasEncryptedToolOutput(body) {
		return
	}
	count := 0
	fingerprints := make([]string, 0, 8)
	openAIRequestPayloadView(body).Get("input").ForEach(func(_, item gjson.Result) bool {
		if kind := item.Get("type").String(); kind != "function_call_output" && kind != "custom_tool_call_output" {
			return true
		}
		if output := item.Get("output"); output.IsArray() {
			output.ForEach(func(_, part gjson.Result) bool {
				encrypted := part.Get("encrypted_content")
				if part.Get("type").String() == "encrypted_content" && encrypted.Type == gjson.String {
					count++
					if len(fingerprints) < cap(fingerprints) {
						digest := sha256.Sum256([]byte(encrypted.Str))
						fingerprints = append(fingerprints, fmt.Sprintf("%x", digest[:12]))
					}
				}
				return true
			})
		}
		return true
	})
	logger.FromContext(ctx).Warn("openai.encrypted_tool_output_rejected",
		zap.Int64("account_id", account.ID),
		zap.Int64("affinity_account_id", openAIEncryptedToolOutputOwner(ctx)),
		zap.Int("encrypted_output_parts", count),
		zap.Strings("encrypted_output_fingerprints", fingerprints))
}

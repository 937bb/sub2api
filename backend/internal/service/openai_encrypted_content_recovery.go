package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/tidwall/gjson"
)

var errOpenAIInvalidEncryptedContent = errors.New("openai request contains invalid encrypted content")

func isOpenAIInvalidEncryptedContentMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "invalid_encrypted_content") ||
		((strings.Contains(message, "encrypted content") || strings.Contains(message, "encrypted_content")) &&
			(strings.Contains(message, "could not be verified") || strings.Contains(message, "could not be decrypted") || strings.Contains(message, "hydration failed")))
}

func isOpenAIInvalidEncryptedContentResponse(status int, body []byte) bool {
	return status == http.StatusBadRequest && (strings.EqualFold(extractUpstreamErrorCode(body), "invalid_encrypted_content") || isOpenAIInvalidEncryptedContentMessage(extractUpstreamErrorMessage(body)))
}

func isOpenAIInvalidEncryptedContentError(err error) bool {
	if errors.Is(err, errOpenAIInvalidEncryptedContent) {
		return true
	}
	var fallback *openAIWSFallbackError
	return (errors.As(err, &fallback) && fallback != nil && strings.TrimPrefix(fallback.Reason, "prewarm_") == openAIWSFallbackReasonInvalidEncryptedContent) || (err != nil && isOpenAIInvalidEncryptedContentMessage(err.Error()))
}

// Only protocol-owned locations are inspected. Tool arguments, user text and
// arbitrary nested JSON must never be recursively scrubbed.
func visitOpenAIEncryptedParts(item gjson.Result, visit func(string)) {
	kind, role := item.Get("type").String(), item.Get("role").String()
	if openAIEncryptedLineageItemType(kind) || (kind == "message" && role == "assistant") {
		if encrypted := item.Get("encrypted_content"); encrypted.Type == gjson.String && encrypted.String() != "" {
			visit(encrypted.String())
		}
	}
	if kind == "encrypted_content" {
		visit(openAIEncryptedFragmentKey(item.Raw))
		return
	}
	if kind != "message" && !(kind == "" && role == "assistant") {
		return
	}
	if role != "" && role != "assistant" {
		return
	}
	item.Get("content").ForEach(func(_, part gjson.Result) bool {
		if part.Get("type").String() == "encrypted_content" {
			visit(openAIEncryptedFragmentKey(part.Raw))
		}
		return true
	})
}

func openAIEncryptedFragmentKey(raw string) string {
	var value any
	if err := decodeOpenAIJSONUseNumber([]byte(raw), &value); err == nil {
		if normalized, err := json.Marshal(value); err == nil {
			raw = string(normalized)
		}
	}
	return "content-fragment:" + raw
}

func sanitizeEncryptedReasoningInputItem(item any) (any, bool, bool) {
	return sanitizeOpenAIEncryptedInputItem(item, nil)
}

// A nil digest set strips all encrypted parts after an explicit rejection or
// known account switch. A non-nil set strips only previously rejected parts.
func sanitizeOpenAIEncryptedInputItem(item any, invalid map[string]struct{}) (any, bool, bool) {
	value, ok := item.(map[string]any)
	if !ok {
		return item, false, true
	}
	matches := func(key string) bool {
		if invalid == nil {
			return true
		}
		_, hit := invalid[openAIEncryptedContentDigest(key)]
		return hit
	}
	kind, _ := value["type"].(string)
	role, _ := value["role"].(string)
	if openAIEncryptedLineageItemType(kind) {
		encrypted, _ := value["encrypted_content"].(string)
		if invalid != nil && !matches(encrypted) {
			return item, false, true
		}
		return sanitizeOpenAIReasoningFields(item)
	}
	if kind == "encrypted_content" {
		raw, _ := json.Marshal(value)
		if matches(openAIEncryptedFragmentKey(string(raw))) {
			return nil, true, false
		}
		return item, false, true
	}
	if (kind != "message" && !(kind == "" && role == "assistant")) || (role != "" && role != "assistant") {
		return item, false, true
	}
	changed := false
	if encrypted, ok := value["encrypted_content"].(string); role == "assistant" && ok && matches(encrypted) {
		delete(value, "encrypted_content")
		changed = true
	}
	if parts, ok := asDeepSeekResponsesSlice(value["content"]); ok {
		kept := make([]any, 0, len(parts))
		for _, part := range parts {
			fragment, ok := part.(map[string]any)
			if ok && fragment["type"] == "encrypted_content" {
				raw, _ := json.Marshal(fragment)
				if matches(openAIEncryptedFragmentKey(string(raw))) {
					changed = true
					continue
				}
			}
			kept = append(kept, part)
		}
		if changed {
			value["content"] = kept
		}
	}
	if !changed {
		return item, false, true
	}
	// A provider-owned message ID can otherwise rehydrate the rejected ciphertext.
	delete(value, "id")
	switch content := value["content"].(type) {
	case []any:
		if len(content) == 0 {
			return nil, true, false
		}
	case nil:
		return nil, true, false
	case string:
		if content == "" {
			return nil, true, false
		}
	}
	return value, true, true
}

// SanitizeOpenAIEncryptedContent preserves plaintext and tool exchanges and
// never mutates the canonical request bytes.
func SanitizeOpenAIEncryptedContent(body []byte) ([]byte, bool, error) {
	if !OpenAIRequestHasEncryptedContent(body) {
		return body, false, nil
	}
	var decoded map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &decoded); err != nil {
		return body, false, err
	}
	target := decoded
	if response, ok := decoded["response"].(map[string]any); ok && strings.HasPrefix(gjson.GetBytes(body, "type").String(), "response.") {
		target = response
	}
	if !trimOpenAIEncryptedReasoningItems(target) {
		return body, false, nil
	}
	result, err := marshalOpenAIUpstreamJSON(decoded)
	return result, err == nil, err
}

func OpenAIRequestHasEncryptedContent(body []byte) bool {
	return len(collectOpenAIEncryptedContentDigestsRaw(body)) > 0
}

type openAIEncryptedAffinityKey struct{}
type openAIEncryptedAffinity struct{ owner atomic.Int64 }

// Snapshot ownership before selection can replace the shared sticky binding.
// Unknown ownership preserves the first attempt; a bounded recovery handles
// actual upstream rejection rather than discarding unproven-invalid context.
func (s *OpenAIGatewayService) WithOpenAIEncryptedContentAffinity(ctx context.Context, groupID *int64, sessionHash, previousResponseID string, body []byte) context.Context {
	if s == nil || !OpenAIRequestHasEncryptedContent(body) {
		return ctx
	}
	affinity := &openAIEncryptedAffinity{}
	var owner int64
	if previousResponseID != "" {
		if store := s.getOpenAIWSStateStore(); store != nil {
			owner, _ = store.GetResponseAccount(ctx, derefGroupID(groupID), previousResponseID)
		}
	}
	if owner == 0 {
		owner, _ = s.getStickySessionAccountID(ctx, groupID, sessionHash)
	}
	affinity.owner.Store(owner)
	return context.WithValue(ctx, openAIEncryptedAffinityKey{}, affinity)
}

func openAIEncryptedContentStickyProtected(ctx context.Context) bool {
	affinity, _ := ctx.Value(openAIEncryptedAffinityKey{}).(*openAIEncryptedAffinity)
	return affinity != nil
}

func SanitizeOpenAIEncryptedContentForAccount(ctx context.Context, account *Account, body []byte) ([]byte, error) {
	affinity, _ := ctx.Value(openAIEncryptedAffinityKey{}).(*openAIEncryptedAffinity)
	if affinity == nil || account == nil || account.Platform != PlatformOpenAI {
		return body, nil
	}
	logicalID := account.ID
	if account.ParentAccountID != nil {
		logicalID = *account.ParentAccountID
	}
	affinity.owner.CompareAndSwap(0, logicalID)
	owner := affinity.owner.Load()
	if owner == account.ID || (account.ParentAccountID != nil && owner == *account.ParentAccountID) {
		return body, nil
	}
	cleaned, _, err := SanitizeOpenAIEncryptedContent(body)
	if err != nil {
		return nil, fmt.Errorf("sanitize cross-account encrypted input: %w", err)
	}
	return cleaned, nil
}

// SchedulingError carries request-level failures from a delivered terminal event
// without changing the successful transport/lifecycle return contract.
func (r *OpenAIForwardResult) SchedulingError() error {
	if r == nil {
		return nil
	}
	return r.schedulingError
}

func openAIEncryptedContentEventError(payload []byte) error {
	if response := gjson.GetBytes(payload, "response"); response.IsObject() {
		payload = []byte(response.Raw)
	}
	if isOpenAIInvalidEncryptedContentResponse(http.StatusBadRequest, payload) {
		return errOpenAIInvalidEncryptedContent
	}
	return nil
}

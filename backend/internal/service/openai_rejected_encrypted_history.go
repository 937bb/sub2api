package service

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var openAIRejectedEncryptedItemID = regexp.MustCompile(`(?i)\bencrypted content for item ['"]?([a-z0-9_-]+)`)
var openAIRejectedEncryptedInputParam = regexp.MustCompile(`^(?:response\.)?input(?:\[([0-9]+)\]|\.([0-9]+))(?:$|\.|\[)`)

func (s *openAIResponsesRejectedFieldRetryState) allowEncryptedHistory(nextBody []byte) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if s.encryptedHistoryRetried {
		s.mu.Unlock()
		return false
	}
	s.encryptedHistoryRetried = true
	s.mu.Unlock()
	return s.Allow(nextBody)
}

// Mixed histories require an explicit rejected item. An unrelated tool result
// must neither prevent reasoning recovery nor be silently discarded with it.
// Sealed calls and tool outputs remain opaque and are never sanitized here.
func sanitizeOpenAIRejectedEncryptedHistory(body, rejection []byte) ([]byte, []string, error) {
	if !isOpenAIInvalidEncryptedContentResponse(400, rejection) {
		return body, nil, nil
	}
	var cleaned []byte
	var err error
	if !openAIRequestHasEncryptedToolOutput(body) {
		cleaned, _, err = SanitizeOpenAIEncryptedContent(body)
	} else {
		message := extractUpstreamErrorMessage(rejection)
		id := ""
		if match := openAIRejectedEncryptedItemID.FindStringSubmatch(message); len(match) > 1 {
			id = match[1]
		}
		index := -1
		param := gjson.GetBytes(rejection, "error.param").String()
		if match := openAIRejectedEncryptedInputParam.FindStringSubmatch(param); len(match) > 1 {
			index, err = strconv.Atoi(match[1] + match[2])
			if err != nil {
				return body, nil, nil
			}
		}
		if id == "" && index < 0 {
			return body, nil, nil
		}
		input := openAIRequestPayloadView(body).Get("input")
		items := input.Array()
		if input.IsObject() {
			items = []gjson.Result{input}
		}
		invalid := make(map[string]struct{})
		matches := 0
		for i, item := range items {
			if (id != "" && item.Get("id").String() != id) || (index >= 0 && i != index) {
				continue
			}
			matches++
			visitOpenAIEncryptedParts(item, func(key string) {
				invalid[openAIEncryptedContentDigest(key)] = struct{}{}
			})
		}
		if matches != 1 || len(invalid) == 0 {
			return body, nil, nil
		}
		cleaned, _, err = stripOpenAIInvalidEncryptedContentRaw(body, invalid)
	}
	if err != nil {
		return body, nil, err
	}
	// Only remember fragments actually removed from this outgoing request.
	// Never mark unrelated, still valid ciphertext from the original history.
	remaining := make(map[string]struct{})
	for _, digest := range collectOpenAIEncryptedContentDigestsRaw(cleaned) {
		remaining[digest] = struct{}{}
	}
	var removed []string
	for _, digest := range collectOpenAIEncryptedContentDigestsRaw(body) {
		if _, exists := remaining[digest]; !exists {
			removed = append(removed, digest)
		}
	}
	return cleaned, removed, nil
}

func (s *OpenAIGatewayService) recoverOpenAIRejectedEncryptedHistory(c *gin.Context, account *Account, body, rejection []byte, turn int) ([]byte, bool, error) {
	cleaned, removed, err := sanitizeOpenAIRejectedEncryptedHistory(body, rejection)
	if err != nil || len(removed) == 0 {
		return body, false, err
	}
	s.markOpenAIWSInvalidEncryptedContentLineage(getOpenAIGroupIDFromContext(c), s.openAIWSLineageSessionHashFromContext(c, body), removed)
	logOpenAIWSModeInfo("invalid_encrypted_history_recovered account_id=%d turn=%d removed_digests=%d", account.ID, turn, len(removed))
	return cleaned, true, nil
}

func openAIEncryptedRejectionFromError(err error) []byte {
	if err == nil {
		return nil
	}
	data, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "invalid_encrypted_content", "message": err.Error()}})
	return data
}

// Agent context is recoverable only when the item retains explicit plaintext.
// An encrypted-only result has no locally available replacement.
func openAIAgentMessageHasPlaintext(item gjson.Result) bool {
	for _, part := range item.Get("content").Array() {
		switch part.Get("type").String() {
		case "input_text", "output_text", "text":
			if strings.TrimSpace(part.Get("text").String()) != "" {
				return true
			}
		}
	}
	return false
}

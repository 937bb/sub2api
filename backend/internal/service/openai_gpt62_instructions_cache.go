package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	openaiapi "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	gpt62InstructionsSessionTTL        = 2 * time.Hour
	gpt62InstructionsSessionMaxEntries = 65536
)

type gpt62InstructionsSessionBinding struct {
	instructionsHash string
	expiresAt        time.Time
}

type gpt62InstructionsSessionCache struct {
	mu      sync.Mutex
	entries map[string]gpt62InstructionsSessionBinding
}

var defaultGPT62InstructionsSessionCache = &gpt62InstructionsSessionCache{
	entries: make(map[string]gpt62InstructionsSessionBinding),
}

func (c *gpt62InstructionsSessionCache) remember(key, instructionsHash string, now time.Time) {
	if c == nil || key == "" || instructionsHash == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= gpt62InstructionsSessionMaxEntries {
		for entryKey, entry := range c.entries {
			if !entry.expiresAt.After(now) {
				delete(c.entries, entryKey)
			}
		}
		if len(c.entries) >= gpt62InstructionsSessionMaxEntries {
			return
		}
	}
	c.entries[key] = gpt62InstructionsSessionBinding{
		instructionsHash: instructionsHash,
		expiresAt:        now.Add(gpt62InstructionsSessionTTL),
	}
}

func (c *gpt62InstructionsSessionCache) matches(key, instructionsHash string, now time.Time) bool {
	if c == nil || key == "" || instructionsHash == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return false
	}
	if !entry.expiresAt.After(now) {
		delete(c.entries, key)
		return false
	}
	return entry.instructionsHash == instructionsHash
}

func gpt62InstructionsSessionKey(root gjson.Result, account *Account) string {
	promptCacheKey := strings.TrimSpace(root.Get("prompt_cache_key").String())
	if promptCacheKey == "" || account == nil {
		return ""
	}
	model := openaiapi.GPT62ModelID(root.Get("model").String())
	if model == "" {
		return ""
	}
	raw := strconv.FormatInt(account.ID, 10) + "|" + model + "|" + promptCacheKey
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func gpt62InstructionsHash(instructions string) string {
	sum := sha256.Sum256([]byte(instructions))
	return hex.EncodeToString(sum[:])
}

func inputStartsWithDeveloperInstructions(input gjson.Result, instructions string) bool {
	if !input.IsArray() {
		return false
	}
	items := input.Array()
	if len(items) == 0 || items[0].Get("role").String() != "developer" {
		return false
	}
	content := items[0].Get("content")
	return content.Type == gjson.String && content.String() == instructions
}

func prependGPT62DeveloperInstructions(body []byte, input gjson.Result, instructions string) ([]byte, error) {
	developerJSON, err := json.Marshal(map[string]any{
		"type":    "message",
		"role":    "developer",
		"content": instructions,
	})
	if err != nil {
		return body, fmt.Errorf("marshal GPT-6.2 developer instructions: %w", err)
	}

	var nextInput []byte
	switch {
	case !input.Exists() || input.Type == gjson.Null:
		nextInput = append(append([]byte{'['}, developerJSON...), ']')
	case input.Type == gjson.String:
		userJSON, marshalErr := json.Marshal(map[string]any{
			"type":    "message",
			"role":    "user",
			"content": input.String(),
		})
		if marshalErr != nil {
			return body, fmt.Errorf("marshal GPT-6.2 user input: %w", marshalErr)
		}
		nextInput = make([]byte, 0, len(developerJSON)+len(userJSON)+3)
		nextInput = append(nextInput, '[')
		nextInput = append(nextInput, developerJSON...)
		nextInput = append(nextInput, ',')
		nextInput = append(nextInput, userJSON...)
		nextInput = append(nextInput, ']')
	case input.IsArray():
		raw := bytes.TrimSpace([]byte(input.Raw))
		if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
			return body, fmt.Errorf("normalize GPT-6.2 instructions: invalid input array")
		}
		inner := bytes.TrimSpace(raw[1 : len(raw)-1])
		nextInput = make([]byte, 0, len(developerJSON)+len(inner)+3)
		nextInput = append(nextInput, '[')
		nextInput = append(nextInput, developerJSON...)
		if len(inner) > 0 {
			nextInput = append(nextInput, ',')
			nextInput = append(nextInput, inner...)
		}
		nextInput = append(nextInput, ']')
	default:
		return body, nil
	}

	next, err := sjson.SetRawBytes(body, "input", nextInput)
	if err != nil {
		return body, fmt.Errorf("set GPT-6.2 developer input: %w", err)
	}
	next, err = sjson.DeleteBytes(next, "instructions")
	if err != nil {
		return body, fmt.Errorf("delete GPT-6.2 top-level instructions: %w", err)
	}
	return next, nil
}

// normalizeGPT62InstructionsForCache follows the GPT-6.2 Responses cache
// layout: stable instructions lead the input as a developer message. A
// continuation only drops repeated top-level instructions when this process
// observed the same prompt_cache_key and instruction bytes on the first turn.
// Unknown or changed continuations retain the original top-level field.
func normalizeGPT62InstructionsForCache(body []byte, account *Account, now time.Time) ([]byte, bool, error) {
	if !usesCodexDeveloperMessages(account) || len(body) == 0 {
		return body, false, nil
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("normalize GPT-6.2 instructions: invalid JSON object")
	}
	if event := root.Get("type"); event.Exists() && event.String() != "response.create" {
		return body, false, nil
	}
	if openaiapi.GPT62ModelID(root.Get("model").String()) == "" {
		return body, false, nil
	}
	instructionsResult := root.Get("instructions")
	if instructionsResult.Type != gjson.String || strings.TrimSpace(instructionsResult.String()) == "" {
		return body, false, nil
	}
	instructions := instructionsResult.String()
	instructionsHash := gpt62InstructionsHash(instructions)
	sessionKey := gpt62InstructionsSessionKey(root, account)
	input := root.Get("input")

	if inputStartsWithDeveloperInstructions(input, instructions) {
		next, err := sjson.DeleteBytes(body, "instructions")
		if err != nil {
			return body, false, fmt.Errorf("delete duplicate GPT-6.2 instructions: %w", err)
		}
		defaultGPT62InstructionsSessionCache.remember(sessionKey, instructionsHash, now)
		return next, true, nil
	}

	previousResponseID := strings.TrimSpace(root.Get("previous_response_id").String())
	if previousResponseID != "" {
		if !defaultGPT62InstructionsSessionCache.matches(sessionKey, instructionsHash, now) {
			return body, false, nil
		}
		next, err := sjson.DeleteBytes(body, "instructions")
		if err != nil {
			return body, false, fmt.Errorf("delete repeated GPT-6.2 instructions: %w", err)
		}
		return next, true, nil
	}

	next, err := prependGPT62DeveloperInstructions(body, input, instructions)
	if err != nil {
		return body, false, err
	}
	if bytes.Equal(next, body) {
		return body, false, nil
	}
	defaultGPT62InstructionsSessionCache.remember(sessionKey, instructionsHash, now)
	return next, true, nil
}

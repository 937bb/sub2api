package kiro

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"
	"math"
	"sort"
	"strings"
)

// PrefixPoint stores metadata only; request text never enters the cache store.
type PrefixPoint struct {
	Digest     string `json:"digest"`
	Tokens     int    `json:"tokens"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type PrefixPlan struct {
	TotalTokens   int
	CoveredTokens int
	Lookups       []PrefixPoint
	Writes        []PrefixPoint
}

type prefixBlock struct {
	signature []byte
	tokens    int
	ttl       int
	cacheable bool
	current   bool
	group     string
}

func prefixFrame(h hash.Hash, data []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(data)
}

func prefixTTL(value any) (int, bool) {
	m, ok := value.(map[string]any)
	if !ok || m["type"] != "ephemeral" {
		return 0, false
	}
	switch m["ttl"] {
	case nil, "5m":
		return 300, true
	case "1h":
		return 3600, true
	default:
		return 0, false
	}
}

// PlanPrefix follows kiro.rs's marked-prefix semantics, including ordered
// tool/system/message blocks, four checkpoints and a 20-position lookback.
// AutoPrefix is a group opt-in when a client does not declare cache_control.
// Malformed cache declarations disable local metering, never the inference.
func PlanPrefix(raw []byte, model string, auto bool, ttl int) *PrefixPlan {
	var err error
	raw, err = ApplyThinkingAlias(raw, model)
	if err != nil {
		return nil
	}
	var typed Request
	if json.Unmarshal(raw, &typed) != nil {
		return nil
	}
	thinkingBoundary := thinkingHistoryBoundary(typed.Messages, model)
	keepThinking := false
	var request map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&request) != nil || request == nil {
		return nil
	}
	var blocks []prefixBlock
	valid := true
	appendBlock := func(section, role string, original any, current bool) {
		content, ok := original.(map[string]any)
		if !ok {
			valid = false
			return
		}
		clean := make(map[string]any, len(content))
		for k, v := range content {
			if k != "cache_control" {
				clean[k] = v
			}
		}
		kind, _ := clean["type"].(string)
		text, _ := clean["text"].(string)
		if kind == "thinking" || kind == "redacted_thinking" {
			// These blocks cannot declare checkpoints. Only retained visible
			// text contributes to prefix identity; signatures never do.
			if _, present := content["cache_control"]; present {
				valid = false
			}
			if section != "message" || role != "assistant" {
				valid = false
				return
			}
			thought, _ := clean["thinking"].(string)
			if kind == "redacted_thinking" || !keepThinking || thought == "" {
				return
			}
			clean = map[string]any{"type": "thinking", "thinking": thought}
		}
		encoded, err := json.Marshal(clean)
		if err != nil {
			valid = false
			return
		}
		tokens := EstimateTokens(string(encoded))
		if kind == "text" {
			tokens = EstimateTokens(text)
		}
		if kind == "thinking" {
			thought, _ := clean["thinking"].(string)
			tokens = EstimateTokens(thinkingHistoryText(thought))
		}
		b := prefixBlock{tokens: tokens, current: current, cacheable: section == "tool" || (kind != "" && kind != "thinking" && kind != "redacted_thinking" && (kind != "text" || strings.TrimSpace(text) != ""))}
		if kind == "tool_use" || kind == "tool_result" {
			b.group = role + ":" + kind
		}
		if control, present := content["cache_control"]; present {
			var ok bool
			b.ttl, ok = prefixTTL(control)
			valid = valid && ok && b.cacheable
		}
		b.signature, _ = json.Marshal(map[string]any{"section": section, "role": role, "content": clean})
		blocks = append(blocks, b)
	}
	appendContent := func(section, role string, content any, current bool) {
		switch value := content.(type) {
		case string:
			appendBlock(section, role, map[string]any{"type": "text", "text": value}, current)
		case []any:
			for _, b := range value {
				appendBlock(section, role, b, current)
			}
		case nil:
		default:
			valid = false
		}
	}
	if tools, ok := request["tools"].([]any); ok {
		for _, tool := range tools {
			appendBlock("tool", "", tool, false)
		}
	}
	appendContent("system", "", request["system"], false)
	messages, _ := request["messages"].([]any)
	// Match control-message merging without mutating the request or its blocks.
	last := -1
	for i, value := range messages {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		if m["role"] != "system" {
			last = i
			continue
		}
		appendContent("system", "", m["content"], false)
		if update, ok := m["output_config"].(map[string]any); ok {
			base, _ := request["output_config"].(map[string]any)
			if base == nil {
				base = map[string]any{}
			}
			for k, v := range update {
				base[k] = v
			}
			request["output_config"] = base
		}
	}
	for i, value := range messages {
		m := value.(map[string]any)
		role, _ := m["role"].(string)
		if role == "system" {
			continue
		}
		keepThinking = role == "assistant" && i >= thinkingBoundary
		current := i == last && role == "user"
		if content, ok := m["content"].([]any); ok {
			for _, v := range content {
				if b, ok := v.(map[string]any); ok && b["type"] == "tool_result" {
					current = false
				}
			}
		}
		appendContent("message", role, m["content"], current)
	}
	if !valid || len(blocks) == 0 || len(blocks) > 8192 {
		return nil
	}
	points := map[int]int{}
	for i, block := range blocks {
		if block.ttl > 0 {
			points[i] = block.ttl
		}
	}
	autoTTL := 0
	if control, present := request["cache_control"]; present {
		var ok bool
		autoTTL, ok = prefixTTL(control)
		if !ok {
			return nil
		}
	} else if auto && len(points) == 0 {
		autoTTL = ttl
	}
	if autoTTL > 0 {
		index := -1
		for i, b := range blocks {
			if b.cacheable && !b.current {
				index = i
			}
		}
		if index < 0 {
			for i, b := range blocks {
				if b.cacheable {
					index = i
				}
			}
		}
		if index >= 0 {
			if existing, ok := points[index]; ok && existing != autoTTL {
				return nil
			}
			points[index] = autoTTL
		}
	}
	if len(points) == 0 || len(points) > 4 {
		return nil
	}
	indices := make([]int, 0, len(points))
	for i := range points {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	seenShort := false
	for _, i := range indices {
		if points[i] > 300 && seenShort {
			return nil
		}
		seenShort = seenShort || points[i] <= 300
	}
	global := map[string]any{"model": ModelID(model)}
	for _, key := range []string{"tool_choice", "thinking", "output_config", "temperature", "stop_sequences"} {
		global[key] = request[key]
	}
	if metadata, ok := request["metadata"].(map[string]any); ok {
		global["user_id"] = metadata["user_id"]
	}
	encoded, _ := json.Marshal(global)
	hasher := sha256.New()
	prefixFrame(hasher, []byte("kiro-prefix-v2-thinking-history"))
	prefixFrame(hasher, encoded)
	plan := &PrefixPlan{}
	all := make([]PrefixPoint, len(blocks))
	for i, b := range blocks {
		prefixFrame(hasher, b.signature)
		plan.TotalTokens += b.tokens
		all[i] = PrefixPoint{Digest: hex.EncodeToString(hasher.Sum(nil)), Tokens: plan.TotalTokens}
	}
	seen := map[string]bool{}
	for _, index := range indices {
		point := all[index]
		point.TTLSeconds = points[index]
		plan.Writes = append(plan.Writes, point)
		plan.CoveredTokens = point.Tokens
		positions := 1
		for i := index; i >= 0; i-- {
			if !seen[all[i].Digest] {
				plan.Lookups = append(plan.Lookups, all[i])
				seen[all[i].Digest] = true
			}
			if i == 0 {
				break
			}
			if blocks[i].group == "" || blocks[i].group != blocks[i-1].group {
				if positions == 20 {
					break
				}
				positions++
			}
		}
	}
	if plan.TotalTokens <= 0 || plan.CoveredTokens <= 0 || len(plan.Lookups) > 2048 {
		return nil
	}
	return plan
}

// Split scales estimates to the response's input total. The group's percentage
// caps local read-token attribution; it never creates a hit without a prefix.
// Known prefixes excluded by that cap are ordinary input, not new writes.
func (p *PrefixPlan) Split(total, matched, percent int) (input, write5m, write1h, read int) {
	if total <= 0 || p == nil || p.TotalTokens <= 0 {
		return max(total, 0), 0, 0, 0
	}
	scale := func(tokens int) int {
		return min(total, max(0, int(math.Round(float64(total)*float64(tokens)/float64(p.TotalTokens)))))
	}
	matched = min(max(matched, 0), p.CoveredTokens)
	read = min(scale(matched), int(int64(total)*int64(min(100, max(0, percent)))/100))
	previous := matched
	for _, point := range p.Writes {
		if point.Tokens <= previous {
			continue
		}
		amount := scale(point.Tokens) - scale(previous)
		if point.TTLSeconds > 300 {
			write1h += amount
		} else {
			write5m += amount
		}
		previous = point.Tokens
	}
	input = total - read - write5m - write1h
	// Keep a nonzero tail for clients that ignore zero-valued usage updates.
	if input == 0 {
		input = 1
		if write5m > 0 {
			write5m--
		} else if write1h > 0 {
			write1h--
		} else {
			read--
		}
	}
	return
}

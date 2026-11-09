package kiro

import (
	"strconv"
	"strings"
)

// Match rs's visible-thinking history policy. Opaque signatures and redacted
// payloads are never replayed: Kiro has no Anthropic signature contract.
func modelKeepsPriorThinking(model string) bool {
	id := strings.ToLower(ModelID(model))
	if !strings.HasPrefix(id, "claude-") {
		return false
	}
	family, version, _ := strings.Cut(strings.TrimPrefix(id, "claude-"), "-")
	if generation, err := strconv.Atoi(family); err == nil {
		return generation >= 5
	}
	numbers := strings.FieldsFunc(version, func(r rune) bool { return r == '.' || r == '-' })
	if len(numbers) == 0 {
		return false
	}
	major, err := strconv.Atoi(numbers[0])
	if err != nil {
		return false
	}
	minor := 0
	if len(numbers) > 1 {
		minor, _ = strconv.Atoi(numbers[1])
	}
	switch family {
	case "opus":
		return major > 4 || (major == 4 && minor >= 5)
	case "sonnet":
		return major > 4 || (major == 4 && minor >= 6)
	default:
		return major >= 5
	}
}

// A nonempty user instruction or image starts a new turn. Tool results and
// whitespace do not end the current tool loop. Callers validate block shapes.
func thinkingHistoryBoundary(messages []Message, model string) int {
	if modelKeepsPriorThinking(model) {
		return 0
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		blocks, err := Blocks(messages[i].Content)
		if err != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_result" || (block.Type == "text" && strings.TrimSpace(block.Text) == "") {
				continue
			}
			return i
		}
	}
	return 0
}

func thinkingHistoryText(text string) string { return "<thinking>" + text + "</thinking>" }

// Keep visible thinking and answer segments in their original order. Adjacent
// text blocks may be citation fragments and must not acquire extra separators.
func assistantHistoryText(blocks []Block, keepThinking bool) string {
	var segments []string
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			segments = append(segments, text.String())
			text.Reset()
		}
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			_, _ = text.WriteString(block.Text)
		case "thinking":
			if block.Thinking == "" {
				continue
			}
			flush()
			if keepThinking {
				segments = append(segments, thinkingHistoryText(block.Thinking))
			}
		case "redacted_thinking":
			flush()
		}
	}
	flush()
	return strings.Join(segments, "\n\n")
}

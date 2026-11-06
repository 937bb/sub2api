package kiro

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxFrameSize = 8 << 20
const maxResponseSize = 32 << 20

// StreamSummary contains counts only; upstream payloads and opaque signatures
// must never be included in errors that can reach logs or clients.
type StreamSummary struct {
	Frames    int
	Assistant int
	Tools     int
	Reasoning int
	Context   int
	Metering  int
	Other     int
}

// ContentFilteredError is emitted only for an explicit upstream stop reason.
// An empty stream alone is never evidence of content filtering.
type ContentFilteredError struct{}

func (*ContentFilteredError) Error() string { return "Kiro stopped generation: CONTENT_FILTERED" }

type NoContentError struct {
	Summary StreamSummary
}

func (e *NoContentError) Error() string {
	s := e.Summary
	return fmt.Sprintf("Kiro returned no content (frames=%d assistant=%d tools=%d reasoning=%d context=%d metering=%d other=%d)", s.Frames, s.Assistant, s.Tools, s.Reasoning, s.Context, s.Metering, s.Other)
}

type Frame struct {
	Headers map[string]string
	Payload []byte
}

func ReadFrame(r io.Reader) (*Frame, error) {
	var prefix [12]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return nil, err
	}
	total := int(binary.BigEndian.Uint32(prefix[0:4]))
	headerLen := int(binary.BigEndian.Uint32(prefix[4:8]))
	if total < 16 || total > maxFrameSize || headerLen > total-16 {
		return nil, errors.New("invalid Kiro frame size")
	}
	if crc32.ChecksumIEEE(prefix[:8]) != binary.BigEndian.Uint32(prefix[8:]) {
		return nil, errors.New("Kiro prelude CRC mismatch")
	}
	frame := make([]byte, total)
	copy(frame, prefix[:])
	if _, err := io.ReadFull(r, frame[12:]); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(frame[:total-4]) != binary.BigEndian.Uint32(frame[total-4:]) {
		return nil, errors.New("Kiro frame CRC mismatch")
	}
	headers := map[string]string{}
	h := frame[12 : 12+headerLen]
	for len(h) > 0 {
		n := int(h[0])
		h = h[1:]
		if n == 0 || len(h) < n+1 {
			return nil, errors.New("invalid Kiro frame header")
		}
		name := string(h[:n])
		kind := h[n]
		h = h[n+1:]
		size := 0
		switch kind {
		case 0, 1:
			size = 0
		case 2:
			size = 1
		case 3:
			size = 2
		case 4:
			size = 4
		case 5, 8:
			size = 8
		case 9:
			size = 16
		case 6, 7:
			if len(h) < 2 {
				return nil, errors.New("truncated Kiro header length")
			}
			size = int(binary.BigEndian.Uint16(h[:2]))
			h = h[2:]
		default:
			return nil, errors.New("unsupported Kiro header type")
		}
		if len(h) < size {
			return nil, errors.New("truncated Kiro header")
		}
		if kind == 7 {
			headers[name] = string(h[:size])
		}
		h = h[size:]
	}
	return &Frame{Headers: headers, Payload: frame[12+headerLen : total-4]}, nil
}

type Usage struct {
	InputTokens              int                 `json:"input_tokens"`
	OutputTokens             int                 `json:"output_tokens"`
	CacheReadInputTokens     int                 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int                 `json:"cache_creation_input_tokens"`
	CacheCreation            *CacheCreationUsage `json:"cache_creation,omitempty"`
	CacheSource              string              `json:"cache_source,omitempty"`
}
type CacheCreationUsage struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}
type Response struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Role         string   `json:"role"`
	Model        string   `json:"model"`
	Content      []Block  `json:"content"`
	StopReason   string   `json:"stop_reason"`
	StopSequence *string  `json:"stop_sequence"`
	Usage        Usage    `json:"usage"`
	Credits      *float64 `json:"-"`
	UsageSource  string   `json:"-"`
}
type Emit func(string, any) error

func emitEvent(emit Emit, name string, data map[string]any) error {
	if emit == nil {
		return nil
	}
	data["type"] = name
	return emit(name, data)
}

// EstimateTokens is explicitly approximate; no synthetic prompt-cache tokens are reported.
func EstimateTokens(s string) int { return (utf8.RuneCountInString(s) + 2) / 3 }
func Decode(r io.Reader, model string, inputTokens int, emit Emit, toolNames ...map[string]string) (*Response, error) {
	return decode(r, model, inputTokens, emit, nil, toolNames...)
}

// Finalize usage after a successful stream, before publishing terminal billing.
func DecodeWithUsageFinalizer(r io.Reader, model string, inputTokens int, emit Emit, finalize func(*Response), toolNames map[string]string) (*Response, error) {
	return decode(r, model, inputTokens, emit, finalize, toolNames)
}

func decode(r io.Reader, model string, inputTokens int, emit Emit, finalize func(*Response), toolNames ...map[string]string) (*Response, error) {
	emit = startOnContent(serialToolEmitter(emit))
	out := &Response{ID: "msg_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Type: "message", Role: "assistant", Model: model, Content: []Block{}, StopReason: "end_turn", Usage: Usage{InputTokens: inputTokens}, UsageSource: "estimated"}
	defer func() {
		if out.UsageSource == "estimated" {
			out.Usage.OutputTokens = 0
			for _, b := range out.Content {
				out.Usage.OutputTokens += EstimateTokens(b.Text + string(b.Input))
			}
		}
	}()
	initialUsage := out.Usage
	if finalize != nil {
		initialUsage = Usage{}
	}
	if err := emitEvent(emit, "message_start", map[string]any{"message": map[string]any{"id": out.ID, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": initialUsage}}); err != nil {
		return out, err
	}
	active := -1
	type toolState struct {
		index  int
		name   string
		input  strings.Builder
		closed bool
	}
	tools := map[string]*toolState{}
	toolOrder := []*toolState{}
	total := 0
	summary := StreamSummary{}
	metered := false
	contentFiltered := false
	closeBlock := func() error {
		if active < 0 {
			return nil
		}
		err := emitEvent(emit, "content_block_stop", map[string]any{"index": active})
		active = -1
		return err
	}
	closeTool := func(tool *toolState) error {
		if tool.closed {
			return nil
		}
		raw := tool.input.String()
		if raw == "" {
			raw = "{}"
		}
		if !objectJSON([]byte(raw)) {
			return errors.New("incomplete or non-object Kiro tool arguments")
		}
		out.Content[tool.index].Input = json.RawMessage(raw)
		tool.closed = true
		return emitEvent(emit, "content_block_stop", map[string]any{"index": tool.index})
	}
	for {
		f, err := ReadFrame(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, fmt.Errorf("read Kiro stream: %w", err)
		}
		total += len(f.Payload)
		summary.Frames++
		if total > maxResponseSize {
			return out, errors.New("Kiro response exceeds size limit")
		}
		if t := f.Headers[":message-type"]; t == "exception" || t == "error" {
			return out, errors.New("Kiro stream returned an upstream error")
		}
		switch f.Headers[":event-type"] {
		case "assistantResponseEvent":
			summary.Assistant++
			var e struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(f.Payload, &e) != nil {
				return out, errors.New("invalid Kiro assistant event")
			}
			if e.Content == "" {
				continue
			}
			if active < 0 {
				active = len(out.Content)
				out.Content = append(out.Content, Block{Type: "text"})
				if err := emitEvent(emit, "content_block_start", map[string]any{"index": active, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
					return out, err
				}
			}
			out.Content[active].Text += e.Content
			if err := emitEvent(emit, "content_block_delta", map[string]any{"index": active, "delta": map[string]any{"type": "text_delta", "text": e.Content}}); err != nil {
				return out, err
			}
		case "toolUseEvent":
			summary.Tools++
			metered = false
			var e struct {
				Name     string `json:"name"`
				ID       string `json:"toolUseId"`
				Input    string `json:"input"`
				Stop     bool   `json:"stop"`
				Complete bool   `json:"isComplete"`
			}
			if json.Unmarshal(f.Payload, &e) != nil || e.ID == "" {
				return out, errors.New("invalid Kiro tool event")
			}
			if err := closeBlock(); err != nil {
				return out, err
			}
			tool := tools[e.ID]
			if tool == nil {
				if e.Name == "" {
					return out, errors.New("Kiro tool name is missing")
				}
				name := e.Name
				if len(toolNames) > 0 && toolNames[0] != nil {
					original, ok := toolNames[0][name]
					if !ok {
						return out, errors.New("Kiro returned an undeclared or disabled tool")
					}
					name = original
				}
				tool = &toolState{index: len(out.Content), name: e.Name}
				tools[e.ID] = tool
				toolOrder = append(toolOrder, tool)
				out.Content = append(out.Content, Block{Type: "tool_use", ID: e.ID, Name: name, Input: json.RawMessage(`{}`)})
				if err := emitEvent(emit, "content_block_start", map[string]any{"index": tool.index, "content_block": out.Content[tool.index]}); err != nil {
					return out, err
				}
			} else if tool.closed || (e.Name != "" && e.Name != tool.name) {
				return out, errors.New("Kiro tool changed name or continued after completion")
			}
			tool.input.WriteString(e.Input)
			if e.Input != "" {
				if err := emitEvent(emit, "content_block_delta", map[string]any{"index": tool.index, "delta": map[string]any{"type": "input_json_delta", "partial_json": e.Input}}); err != nil {
					return out, err
				}
			}
			if e.Stop || e.Complete {
				if err := closeTool(tool); err != nil {
					return out, err
				}
			}
			out.StopReason = "tool_use"
		case "meteringEvent":
			summary.Metering++
			metered = true
			var e struct {
				Unit   string   `json:"unit"`
				Usage  *float64 `json:"usage"`
				Input  *int     `json:"inputTokens"`
				Output *int     `json:"outputTokens"`
				Read   *int     `json:"cacheReadInputTokens"`
				Write  *int     `json:"cacheCreationInputTokens"`
			}
			if json.Unmarshal(f.Payload, &e) != nil {
				return out, errors.New("invalid Kiro metering event")
			}
			if strings.Contains(strings.ToLower(e.Unit), "credit") {
				out.Credits = e.Usage
			}
			if e.Input != nil && e.Output != nil && *e.Input >= 0 && *e.Output >= 0 {
				out.Usage.InputTokens = *e.Input
				out.Usage.OutputTokens = *e.Output
				out.UsageSource = "upstream"
			}
			if e.Read != nil && *e.Read >= 0 {
				out.Usage.CacheReadInputTokens = *e.Read
			}
			if e.Write != nil && *e.Write >= 0 {
				out.Usage.CacheCreationInputTokens = *e.Write
			}
		case "metadataEvent":
			var e struct {
				StopReason string `json:"stopReason"`
			}
			if json.Unmarshal(f.Payload, &e) != nil {
				return out, errors.New("invalid Kiro metadata event")
			}
			switch e.StopReason {
			case "CONTENT_FILTERED":
				contentFiltered = true
			case "MAX_TOKENS":
				out.StopReason = "max_tokens"
			}
		case "reasoningContentEvent":
			summary.Reasoning++
		case "contextUsageEvent":
			summary.Context++
		default:
			summary.Other++
		}
	}
	if contentFiltered {
		return out, &ContentFilteredError{}
	}
	for _, tool := range toolOrder {
		if !tool.closed {
			// Some IDE streams use metering as terminal evidence instead of stop=true.
			if !metered {
				return out, errors.New("Kiro stream ended before tool completion")
			}
			if err := closeTool(tool); err != nil {
				return out, err
			}
		}
	}
	if err := closeBlock(); err != nil {
		return out, err
	}
	if len(out.Content) == 0 {
		return out, &NoContentError{Summary: summary}
	}
	if out.UsageSource == "estimated" {
		for _, b := range out.Content {
			out.Usage.OutputTokens += EstimateTokens(b.Text + string(b.Input))
		}
	}
	if finalize != nil {
		finalize(out)
	}
	if err := emitEvent(emit, "message_delta", map[string]any{"delta": map[string]any{"stop_reason": out.StopReason, "stop_sequence": nil}, "usage": out.Usage}); err != nil {
		return out, err
	}
	if err := emitEvent(emit, "message_stop", map[string]any{}); err != nil {
		return out, err
	}
	return out, nil
}

// Do not publish a successful response envelope before receiving content.
// Context, metering and reasoning-only streams may end without an answer.
func startOnContent(emit Emit) Emit {
	if emit == nil {
		return nil
	}
	var start any
	return func(name string, data any) error {
		if name == "message_start" {
			start = data
			return nil
		}
		if start != nil {
			if err := emit("message_start", start); err != nil {
				return err
			}
			start = nil
		}
		return emit(name, data)
	}
}

// Replay emits only previously completed, text-only responses. No upstream tokens are charged.
func Replay(out *Response, emit Emit) error {
	if err := emitEvent(emit, "message_start", map[string]any{"message": map[string]any{"id": out.ID, "type": "message", "role": "assistant", "model": out.Model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": out.Usage}}); err != nil {
		return err
	}
	for i, b := range out.Content {
		if b.Type != "text" {
			return errors.New("non-text response is not replayable")
		}
		if err := emitEvent(emit, "content_block_start", map[string]any{"index": i, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
			return err
		}
		if err := emitEvent(emit, "content_block_delta", map[string]any{"index": i, "delta": map[string]any{"type": "text_delta", "text": b.Text}}); err != nil {
			return err
		}
		if err := emitEvent(emit, "content_block_stop", map[string]any{"index": i}); err != nil {
			return err
		}
	}
	if err := emitEvent(emit, "message_delta", map[string]any{"delta": map[string]any{"stop_reason": out.StopReason, "stop_sequence": nil}, "usage": out.Usage}); err != nil {
		return err
	}
	return emitEvent(emit, "message_stop", map[string]any{})
}

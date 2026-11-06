package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
)

func (s *GatewayService) ForwardKiroGemini(ctx context.Context, c *gin.Context, a *Account, model, action string, stream bool, body []byte) (*ForwardResult, error) {
	start := time.Now()
	reject := func(err error) (*ForwardResult, error) {
		c.JSON(400, gin.H{"error": gin.H{"code": 400, "status": "INVALID_ARGUMENT", "message": err.Error()}})
		return nil, err
	}
	if action != "generateContent" && action != "streamGenerateContent" {
		return reject(fmt.Errorf("Kiro only supports generateContent and streamGenerateContent"))
	}
	raw, err := kiro.GeminiRequest(body, model)
	if err != nil {
		return reject(err)
	}
	parsed, err := kiroParsedRequest(c, raw)
	if err != nil {
		return reject(err)
	}
	mapped := a.GetMappedModel(model)
	resp, err := s.kiroAsAnthropic(ctx, c, a, raw, parsed, mapped)
	if err != nil {
		writeKiroEmptyResponse(c, err, "gemini")
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	result := &ForwardResult{Model: model, UpstreamModel: mapped, UpstreamResponseModel: mapped, Stream: stream}
	parts := []any{}
	blocks := map[int]*apicompat.AnthropicContentBlock{}
	args := map[int]string{}
	done := false
	finish := "STOP"
	emit := func(items []any, terminal bool) error {
		candidate := gin.H{"index": 0, "content": gin.H{"role": "model", "parts": items}}
		message := gin.H{"candidates": []any{candidate}, "modelVersion": model, "responseId": result.RequestID}
		if terminal {
			candidate["finishReason"] = finish
			prompt := result.Usage.InputTokens + result.Usage.CacheReadInputTokens + result.Usage.CacheCreationInputTokens
			message["usageMetadata"] = gin.H{"promptTokenCount": prompt, "candidatesTokenCount": result.Usage.OutputTokens, "totalTokenCount": prompt + result.Usage.OutputTokens, "cachedContentTokenCount": result.Usage.CacheReadInputTokens, "cacheCreationInputTokens": result.Usage.CacheCreationInputTokens}
		}
		if stream {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("X-Accel-Buffering", "no")
			data, e := json.Marshal(message)
			if e != nil {
				return e
			}
			_, e = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			return e
		}
		if terminal {
			c.JSON(200, message)
		}
		return nil
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 65536), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event apicompat.AnthropicStreamEvent
		if e := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); e != nil {
			return result, e
		}
		index := 0
		if event.Index != nil {
			index = *event.Index
		}
		switch event.Type {
		case "message_start":
			if event.Message != nil {
				result.RequestID = event.Message.ID
				mergeAnthropicUsage(&result.Usage, event.Message.Usage)
			}
		case "content_block_start":
			if event.ContentBlock != nil {
				blocks[index] = event.ContentBlock
				if event.ContentBlock.Type == "text" && event.ContentBlock.Text != "" {
					item := gin.H{"text": event.ContentBlock.Text}
					parts = append(parts, item)
					if stream {
						if e := emit([]any{item}, false); e != nil {
							return result, e
						}
					}
				}
			}
		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			if result.FirstTokenMs == nil {
				ms := int(time.Since(start).Milliseconds())
				result.FirstTokenMs = &ms
			}
			switch event.Delta.Type {
			case "text_delta":
				item := gin.H{"text": event.Delta.Text}
				parts = append(parts, item)
				if stream {
					if e := emit([]any{item}, false); e != nil {
						return result, e
					}
				}
			case "input_json_delta":
				args[index] += event.Delta.PartialJSON
			default:
				return result, fmt.Errorf("unsupported Kiro Gemini event")
			}
		case "content_block_stop":
			if block := blocks[index]; block != nil && block.Type == "tool_use" {
				var input any
				data := block.Input
				if args[index] != "" {
					data = []byte(args[index])
				}
				if len(data) == 0 {
					data = []byte(`{}`)
				}
				if json.Unmarshal(data, &input) != nil {
					return result, fmt.Errorf("invalid Kiro function arguments")
				}
				item := gin.H{"functionCall": gin.H{"id": block.ID, "name": block.Name, "args": input}}
				parts = append(parts, item)
				if stream {
					if e := emit([]any{item}, false); e != nil {
						return result, e
					}
				}
			}
		case "message_delta":
			if event.Usage != nil {
				mergeAnthropicUsage(&result.Usage, *event.Usage)
			}
			if event.Delta != nil && event.Delta.StopReason == "max_tokens" {
				finish = "MAX_TOKENS"
			}
		case "message_stop":
			done = true
		case "error":
			return result, fmt.Errorf("Kiro stream interrupted")
		}
	}
	result.Duration = time.Since(start)
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if !done {
		return result, fmt.Errorf("Kiro stream ended without terminal event")
	}
	if stream {
		return result, emit([]any{}, true)
	}
	return result, emit(parts, true)
}

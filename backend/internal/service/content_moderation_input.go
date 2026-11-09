package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/tidwall/gjson"
)

func ExtractContentModerationText(protocol string, body []byte) string {
	return ExtractContentModerationInput(protocol, body).Text
}

func ExtractContentModerationInput(protocol string, body []byte) ContentModerationInput {
	return extractContentModerationInput(protocol, body, true)
}

// Keyword checks retain the latest user-text boundary. Tool data is sent to
// semantic moderation, not substring keyword rules. All client text is untrusted.
func extractContentModerationKeywordText(protocol string, body []byte) string {
	return extractContentModerationInput(protocol, body, false).Text
}

type moderationTextCollector struct {
	includeTools bool
}

func extractContentModerationInput(protocol string, body []byte, includeTools bool) ContentModerationInput {
	collector := moderationTextCollector{includeTools: includeTools}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ContentModerationInput{}
	}
	// WS response.create may carry the request under response. Root input wins,
	// matching the protocol snapshot reader, so both gates audit the same data.
	if (protocol == ContentModerationProtocolOpenAIResponses || protocol == "responses_websocket") && gjson.GetBytes(body, "type").String() == "response.create" && !gjson.GetBytes(body, "input").Exists() {
		if response := gjson.GetBytes(body, "response"); response.IsObject() {
			body = []byte(response.Raw)
		}
	}
	var parts []string
	var images []string
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collector.collectLastAnthropicUserMessage(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIChat:
		collector.collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
	case ContentModerationProtocolOpenAIResponses, "responses_websocket":
		collector.collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
	case ContentModerationProtocolGemini:
		collector.collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	case ContentModerationProtocolOpenAIImages:
		collector.addModerationText(&parts, gjson.GetBytes(body, "prompt").String())
		collector.collectContentValue(gjson.GetBytes(body, "images"), &parts, &images)
	case ContentModerationProtocolTypeSafeSystemOne:
		// System One carries no client-harness reminder blocks, so a literal
		// <system-reminder> is ordinary user text and must never be skipped.
		moderationTextCollector{}.collectSystemOneInput(body, &parts)
	default:
		collector.collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
		collector.collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
		collector.collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	}
	out := ContentModerationInput{
		Text:   normalizeContentModerationText(strings.Join(parts, "\n")),
		Images: normalizeModerationImages(images),
	}
	if includeTools {
		out.Normalize()
	}
	return out
}

// collectSystemOneInput moderates every client-controlled text of a System One
// request: question IDs, every question field except the validated type,
// unknown top-level extension fields, and the evaluated state. Object keys are
// sent to Jev as part of the JSON, so they are moderated like values.
func (collector moderationTextCollector) collectSystemOneInput(body []byte, parts *[]string) {
	root := gjson.ParseBytes(body)
	questions := root.Get("questions")
	if !questions.IsObject() {
		collector.collectSystemOneText(questions, parts)
	}
	questions.ForEach(func(id, question gjson.Result) bool {
		collector.addModerationText(parts, id.String())
		if !question.IsObject() {
			collector.collectSystemOneText(question, parts)
			return true
		}
		question.ForEach(func(field, value gjson.Result) bool {
			switch field.String() {
			case "type":
				return true
			case "instructions", "criteria":
			default:
				collector.addModerationText(parts, field.String())
			}
			collector.collectSystemOneText(value, parts)
			return true
		})
		return true
	})
	root.ForEach(func(field, value gjson.Result) bool {
		switch field.String() {
		case "model", "stream", "state", "questions":
			return true
		}
		collector.addModerationText(parts, field.String())
		collector.collectSystemOneText(value, parts)
		return true
	})
	collector.collectSystemOneText(root.Get("state"), parts)
}

func (collector moderationTextCollector) collectSystemOneText(value gjson.Result, parts *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		collector.addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, child gjson.Result) bool {
			collector.collectSystemOneText(child, parts)
			return true
		})
	case value.IsObject():
		value.ForEach(func(key, child gjson.Result) bool {
			collector.addModerationText(parts, key.String())
			collector.collectSystemOneText(child, parts)
			return true
		})
	}
}

func (collector moderationTextCollector) collectLastRoleMessage(messages gjson.Result, role string, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	start := len(array) - 1
	seenUser := false
	for start >= 0 {
		r := strings.ToLower(strings.TrimSpace(array[start].Get("role").String()))
		if r != role && !(collector.includeTools && (r == "tool" || r == "function")) {
			break
		}
		if r == role {
			if seenUser {
				break
			}
			seenUser = true
		}
		start--
		if !collector.includeTools {
			break
		}
	}
	for _, item := range array[start+1:] {
		collector.collectContentValue(item.Get("content"), parts, images)
	}
}

func (collector moderationTextCollector) collectLastAnthropicUserMessage(messages gjson.Result, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	// Anthropic requests may append a system message after the user turn. It
	// does not represent a new model/tool turn, so retain the latest user
	// content; tool_result blocks in the latest user turn are audited below.
	lastUser := len(array) - 1
	for lastUser >= 0 && strings.ToLower(strings.TrimSpace(array[lastUser].Get("role").String())) == "system" {
		lastUser--
	}
	if lastUser < 0 || strings.ToLower(strings.TrimSpace(array[lastUser].Get("role").String())) != "user" {
		return
	}
	start := lastUser
	for collector.includeTools && start > 0 && strings.EqualFold(array[start-1].Get("role").String(), "user") {
		// Earlier ordinary user messages retain the existing latest-message boundary.
		if !hasAnthropicModerationToolResult(array[start-1].Get("content")) {
			break
		}
		start--
	}
	for _, item := range array[start : lastUser+1] {
		collector.collectAnthropicUserContentValue(item.Get("content"), parts, images)
	}
}

func hasAnthropicModerationToolResult(content gjson.Result) bool {
	if content.Get("type").String() == "tool_result" {
		return true
	}
	for _, item := range content.Array() {
		if item.Get("type").String() == "tool_result" {
			return true
		}
	}
	return false
}

func (collector moderationTextCollector) collectAnthropicUserContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		collector.addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collector.collectAnthropicUserContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				collector.addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collector.collectAnthropicUserContentValue(value.Get("content"), parts, images)
			}
		case "tool_result":
			if collector.includeTools {
				moderationTextCollector{}.collectContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
			collector.collectContentValue(value, parts, images)
		}
	}
}

func (collector moderationTextCollector) collectLastResponsesInput(input gjson.Result, parts *[]string, images *[]string) {
	if input.Type == gjson.String {
		collector.addModerationText(parts, input.String())
		return
	}
	var items []gjson.Result
	if input.IsArray() {
		items = input.Array()
	} else if input.IsObject() {
		items = []gjson.Result{input}
	}
	start := len(items) - 1
	seenUser := false
	for start >= 0 {
		item := items[start]
		if !collector.isResponsesUserTextItem(item) && !(collector.includeTools && isModerationToolOutput(item)) {
			break
		}
		if !isModerationToolOutput(item) {
			if seenUser {
				break
			}
			seenUser = true
		}
		start--
		if !collector.includeTools {
			break
		}
	}
	for _, item := range items[start+1:] {
		if isModerationToolOutput(item) {
			if collector.includeTools {
				moderationTextCollector{}.collectToolOutput(item.Get("output"), parts, images)
				if item.Get("type").String() == "tool_search_output" {
					moderationTextCollector{}.collectToolOutput(item.Get("tools"), parts, images)
				}
			}
			continue
		}
		if item.Get("text").Exists() {
			collector.collectContentValue(item, parts, images)
		} else {
			collector.collectContentValue(item.Get("content"), parts, images)
		}
	}
}

func isModerationToolOutput(item gjson.Result) bool {
	switch item.Get("type").String() {
	case "function_call_output", "custom_tool_call_output", "tool_search_output":
		return true
	default:
		return false
	}
}

// Tool output is untrusted data; reminder markers cannot hide it from moderation.
// Preserve structured JSON values, including numbers, instead of dropping them.
func (collector moderationTextCollector) collectToolOutput(value gjson.Result, parts *[]string, images *[]string) {
	if !value.Exists() {
		return
	}
	if value.IsArray() {
		value.ForEach(func(_, child gjson.Result) bool { collector.collectToolOutput(child, parts, images); return true })
		return
	}
	if value.IsObject() {
		switch value.Get("type").String() {
		case "text", "input_text", "message", "image", "input_image", "image_url":
		default:
			collector.addModerationText(parts, value.Raw)
			return
		}
	}
	collector.collectContentValue(value, parts, images)
}

func (collector moderationTextCollector) isResponsesUserTextItem(item gjson.Result) bool {
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	if role == "user" {
		return collector.responseItemHasModerationText(item)
	}
	if role != "" {
		return false
	}
	return collector.responseItemHasModerationText(item)
}

func (collector moderationTextCollector) responseItemHasModerationText(item gjson.Result) bool {
	var parts []string
	var images []string
	collector.collectContentValue(item.Get("content"), &parts, &images)
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collector.collectContentValue(item, &parts, &images)
	}
	return normalizeContentModerationText(strings.Join(parts, "\n")) != "" || len(images) > 0
}

func (collector moderationTextCollector) collectLastGeminiContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
	}
	array := contents.Array()
	start := len(array) - 1
	for start >= 0 {
		role := strings.ToLower(strings.TrimSpace(array[start].Get("role").String()))
		if role != "" && role != "user" && !(collector.includeTools && role == "function") {
			break
		}
		if start < len(array)-1 && !hasGeminiModerationToolResult(array[start].Get("parts")) {
			break
		}
		start--
		if !collector.includeTools {
			break
		}
	}
	for _, item := range array[start+1:] {
		item.Get("parts").ForEach(func(_, part gjson.Result) bool {
			collector.addModerationText(parts, part.Get("text").String())
			addGeminiModerationImage(images, part)
			if collector.includeTools {
				moderationTextCollector{}.collectToolOutput(part.Get("functionResponse.response"), parts, images)
				moderationTextCollector{}.collectToolOutput(part.Get("function_response.response"), parts, images)
			}
			return true
		})
	}
}

func hasGeminiModerationToolResult(parts gjson.Result) bool {
	for _, part := range parts.Array() {
		if part.Get("functionResponse").Exists() || part.Get("function_response").Exists() {
			return true
		}
	}
	return false
}

func (collector moderationTextCollector) collectContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		collector.addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collector.collectContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		addModerationImage(images, value.Get("image_url.url").String())
		addModerationImage(images, value.Get("image_url").String())
		addModerationImage(images, value.Get("url").String())
		addModerationImageData(images, value.Get("source.media_type").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("source.mediaType").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("data").String())
		addModerationImage(images, value.Get("source.data").String())
		addModerationImage(images, value.Get("data").String())
		addModerationImage(images, value.Get("base64").String())
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				collector.addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collector.collectContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
		}
	}
}

func addGeminiModerationImage(images *[]string, part gjson.Result) {
	if inlineData := part.Get("inline_data"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mime_type").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	if inlineData := part.Get("inlineData"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mimeType").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	addModerationImage(images, part.Get("file_data.file_uri").String())
	addModerationImage(images, part.Get("fileData.fileUri").String())
}

func addModerationImageData(images *[]string, mimeType string, data string) {
	mimeType = strings.TrimSpace(mimeType)
	data = strings.TrimSpace(data)
	if mimeType == "" || data == "" {
		return
	}
	addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
}

func addModerationImage(images *[]string, image string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return
	}
	if strings.HasPrefix(image, "data:") || strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		*images = append(*images, image)
	}
}

func normalizeModerationImages(images []string) []string {
	out := make([]string, 0, len(images))
	seen := make(map[string]struct{}, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if _, ok := seen[image]; ok {
			continue
		}
		seen[image] = struct{}{}
		out = append(out, image)
	}
	return out
}

func limitContentModerationImages(images []string) []string {
	if len(images) <= maxContentModerationInputImages {
		return images
	}
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(images))))
	if err != nil {
		return images[:maxContentModerationInputImages]
	}
	return []string{images[int(idx.Int64())]}
}

func (collector moderationTextCollector) addModerationText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	*parts = append(*parts, text)
}

func normalizeContentModerationText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

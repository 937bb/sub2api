package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// invalid_encrypted_content 失效密文 lineage。
//
// 上游判定某轮请求中的加密 reasoning/compaction 项不可解后，同一失效密文会随
// 客户端维护的会话历史在后续每一轮重新出现，重复触发"整包被拒→剥离→重试/
// 重连"。这里按会话记录已被上游拒绝过的 encrypted_content 摘要
// （OpenAIWSStateStore，带 TTL 与容量自保护）；后续请求进场时仅剥离摘要命中
// 的项，新生成的密文摘要不同，不会被误删。

const openAIWSFallbackReasonInvalidEncryptedContent = "invalid_encrypted_content"

const openAIEncryptedToolOutputOmitted = "[Encrypted tool output omitted because it belongs to a different upstream account.]"

// openAIWSIngressSessionHashContextKey 在 gin context 中携带 ingress 会话哈希，
// 供 HTTP bridge turn 内的 lineage 记录复用同一会话键。
const openAIWSIngressSessionHashContextKey = "openai_ws_ingress_session_hash"

func openAIEncryptedContentDigest(encrypted string) string {
	sum := sha256.Sum256([]byte(encrypted))
	return hex.EncodeToString(sum[:])
}

// openAIEncryptedLineageItemType 限定 lineage 覆盖的项类型，与剥离端
// sanitizeEncryptedReasoningInputItem 能处理的类型对称；其他类型即使携带
// encrypted_content 也不收摘要，避免记入永远剥不掉的摘要后每轮空转解码。
func openAIEncryptedLineageItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "reasoning", "compaction", "compaction_summary":
		return true
	default:
		return false
	}
}

func openAIEncryptedContentValue(part map[string]any) string {
	if part == nil {
		return ""
	}
	encrypted, _ := part["encrypted_content"].(string)
	if strings.TrimSpace(encrypted) == "" {
		return ""
	}
	return encrypted
}

func openAIEncryptedToolOutputItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}

func openAIEncryptedAgentItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "item_reference", "agent_message":
		return true
	default:
		return false
	}
}

func openAIEncryptedAgentItemDigest(id string) string {
	return openAIEncryptedContentDigest("agent_item:" + strings.TrimSpace(id))
}

func openAIEncryptedAgentMessageMatches(
	item map[string]any,
	shouldMatch func(string) bool,
) bool {
	if item == nil || strings.TrimSpace(stringValue(item["type"])) != "agent_message" {
		return false
	}
	matchPart := func(raw any) bool {
		part, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(part["type"])) != "encrypted_content" {
			return false
		}
		encrypted := openAIEncryptedContentValue(part)
		return encrypted != "" && shouldMatch != nil && shouldMatch(encrypted)
	}
	switch content := item["content"].(type) {
	case []any:
		for _, raw := range content {
			if matchPart(raw) {
				return true
			}
		}
	case map[string]any:
		return matchPart(content)
	}
	return false
}

func replaceOpenAIEncryptedToolOutputParts(
	item map[string]any,
	shouldReplace func(string) bool,
) int {
	if item == nil || !openAIEncryptedToolOutputItemType(stringValue(item["type"])) {
		return 0
	}
	replacement := func() map[string]any {
		return map[string]any{
			"type": "input_text",
			"text": openAIEncryptedToolOutputOmitted,
		}
	}
	replacePart := func(raw any) (any, bool) {
		part, ok := raw.(map[string]any)
		if !ok {
			return raw, false
		}
		encrypted := openAIEncryptedContentValue(part)
		if encrypted == "" || (shouldReplace != nil && !shouldReplace(encrypted)) {
			return raw, false
		}
		return replacement(), true
	}

	output, exists := item["output"]
	if !exists {
		return 0
	}
	switch parts := output.(type) {
	case []any:
		replaced := 0
		for index, raw := range parts {
			next, changed := replacePart(raw)
			if !changed {
				continue
			}
			parts[index] = next
			replaced++
		}
		return replaced
	case map[string]any:
		next, changed := replacePart(parts)
		if !changed {
			return 0
		}
		item["output"] = next
		return 1
	default:
		return 0
	}
}

func trimOpenAIEncryptedToolOutputParts(reqBody map[string]any) int {
	if len(reqBody) == 0 {
		return 0
	}
	input, exists := reqBody["input"]
	if !exists {
		return 0
	}
	replaced := 0
	visit := func(raw any) {
		item, ok := raw.(map[string]any)
		if !ok {
			return
		}
		replaced += replaceOpenAIEncryptedToolOutputParts(item, nil)
	}
	switch items := input.(type) {
	case []any:
		for _, item := range items {
			visit(item)
		}
	case map[string]any:
		visit(items)
	}
	return replaced
}

func appendOpenAIEncryptedContentDigestsFromItem(item gjson.Result, digests *[]string) {
	if digests == nil || !item.IsObject() {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	if openAIEncryptedLineageItemType(itemType) {
		encrypted := item.Get("encrypted_content")
		if encrypted.Type == gjson.String && encrypted.String() != "" {
			*digests = append(*digests, openAIEncryptedContentDigest(encrypted.String()))
		}
	}
	if !openAIEncryptedToolOutputItemType(itemType) {
		return
	}
	output := item.Get("output")
	appendPart := func(part gjson.Result) {
		encrypted := part.Get("encrypted_content")
		if encrypted.Type == gjson.String && encrypted.String() != "" {
			*digests = append(*digests, openAIEncryptedContentDigest(encrypted.String()))
		}
	}
	if output.IsArray() {
		output.ForEach(func(_, part gjson.Result) bool {
			appendPart(part)
			return true
		})
	} else if output.IsObject() {
		appendPart(output)
	}
}

func appendOpenAIEncryptedAgentItemDigestsFromItem(item gjson.Result, digests *[]string) {
	if digests == nil || !item.IsObject() || !openAIEncryptedAgentItemType(item.Get("type").String()) {
		return
	}
	id := strings.TrimSpace(item.Get("id").String())
	if strings.HasPrefix(id, "amsg_") {
		*digests = append(*digests, openAIEncryptedAgentItemDigest(id))
	}
	if strings.TrimSpace(item.Get("type").String()) != "agent_message" {
		return
	}
	appendPart := func(part gjson.Result) {
		if strings.TrimSpace(part.Get("type").String()) != "encrypted_content" {
			return
		}
		encrypted := part.Get("encrypted_content")
		if encrypted.Type == gjson.String && encrypted.String() != "" {
			*digests = append(*digests, openAIEncryptedContentDigest(encrypted.String()))
		}
	}
	content := item.Get("content")
	if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			appendPart(part)
			return true
		})
	} else if content.IsObject() {
		appendPart(content)
	}
}

// collectOpenAIRejectedAgentItemDigestsRaw records only the amsg_* item IDs
// explicitly named by the upstream error. These synthetic digests let later
// turns remove the same stale reference before their first upstream attempt.
func collectOpenAIRejectedAgentItemDigestsRaw(payload, errorBody []byte) []string {
	if len(payload) == 0 || len(errorBody) == 0 {
		return nil
	}
	input := gjson.Get(openAIWSPayloadStringView(payload), "input")
	if !input.Exists() {
		return nil
	}
	var digests []string
	appendItem := func(item gjson.Result) {
		if !openAIEncryptedAgentItemType(item.Get("type").String()) {
			return
		}
		id := strings.TrimSpace(item.Get("id").String())
		if !strings.HasPrefix(id, "amsg_") || !strings.Contains(string(errorBody), id) {
			return
		}
		appendOpenAIEncryptedAgentItemDigestsFromItem(item, &digests)
	}
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			appendItem(item)
			return true
		})
	} else if input.IsObject() {
		appendItem(input)
	}
	return digests
}

// collectOpenAIEncryptedContentDigestsRaw 收集 input 中 lineage 覆盖类型所携
// 密文的摘要。payload 须遵守 replay 所有权不变式（零拷贝视图解析）。
func collectOpenAIEncryptedContentDigestsRaw(payload []byte) []string {
	if len(payload) == 0 {
		return nil
	}
	input := gjson.Get(openAIWSPayloadStringView(payload), "input")
	if !input.Exists() {
		return nil
	}
	var digests []string
	appendItem := func(item gjson.Result) {
		appendOpenAIEncryptedContentDigestsFromItem(item, &digests)
	}
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			appendItem(item)
			return true
		})
		return digests
	}
	if input.IsObject() {
		appendItem(input)
	}
	return digests
}

// stripOpenAIInvalidEncryptedContentItems 剥离 input 中摘要命中 invalid 的密文
// 项，剥离语义与 trimOpenAIEncryptedReasoningItems 对齐：reasoning 仅剥
// encrypted_content（连带 content:null 清理与空骨架删除），compaction/
// compaction_summary 整项删除。返回被改写的项数。
func stripOpenAIInvalidEncryptedContentItems(reqBody map[string]any, invalid map[string]struct{}) int {
	if len(reqBody) == 0 || len(invalid) == 0 {
		return 0
	}
	inputValue, has := reqBody["input"]
	if !has {
		return 0
	}
	stripped := 0
	stripItem := func(item any) (next any, changedCount int, keep bool) {
		inputItem, ok := item.(map[string]any)
		if !ok {
			return item, 0, true
		}
		itemType := strings.TrimSpace(stringValue(inputItem["type"]))
		if openAIEncryptedAgentItemType(itemType) {
			id := strings.TrimSpace(stringValue(inputItem["id"]))
			if strings.HasPrefix(id, "amsg_") {
				if _, hit := invalid[openAIEncryptedAgentItemDigest(id)]; hit {
					return nil, 1, false
				}
			}
			if itemType == "agent_message" && openAIEncryptedAgentMessageMatches(inputItem, func(encrypted string) bool {
				_, hit := invalid[openAIEncryptedContentDigest(encrypted)]
				return hit
			}) {
				return nil, 1, false
			}
		}
		if openAIEncryptedToolOutputItemType(itemType) {
			replaced := replaceOpenAIEncryptedToolOutputParts(inputItem, func(encrypted string) bool {
				_, hit := invalid[openAIEncryptedContentDigest(encrypted)]
				return hit
			})
			return inputItem, replaced, true
		}
		encrypted, ok := inputItem["encrypted_content"].(string)
		if !ok || encrypted == "" {
			return item, 0, true
		}
		if _, hit := invalid[openAIEncryptedContentDigest(encrypted)]; !hit {
			return item, 0, true
		}
		nextItem, changed, keep := sanitizeEncryptedReasoningInputItem(item)
		if !changed {
			return item, 0, true
		}
		return nextItem, 1, keep
	}
	switch input := inputValue.(type) {
	case []any:
		filtered := input[:0]
		for _, item := range input {
			nextItem, changedCount, keep := stripItem(item)
			stripped += changedCount
			if !keep {
				continue
			}
			filtered = append(filtered, nextItem)
		}
		if stripped == 0 {
			return 0
		}
		if len(filtered) == 0 {
			delete(reqBody, "input")
			return stripped
		}
		reqBody["input"] = filtered
		return stripped
	case map[string]any:
		nextItem, changedCount, keep := stripItem(input)
		if changedCount == 0 {
			return 0
		}
		if !keep {
			delete(reqBody, "input")
			return changedCount
		}
		nextMap, ok := nextItem.(map[string]any)
		if !ok {
			return 0
		}
		reqBody["input"] = nextMap
		return changedCount
	default:
		return 0
	}
}

// openAIRawPayloadHasInvalidEncryptedContent 探测 raw payload 是否携带命中
// invalid 摘要的密文项；零命中的常态路径不做任何改写。payload 须遵守 replay
// 所有权不变式（零拷贝视图解析）。
func openAIRawPayloadHasInvalidEncryptedContent(payload []byte, invalid map[string]struct{}) bool {
	if len(payload) == 0 || len(invalid) == 0 {
		return false
	}
	input := gjson.Get(openAIWSPayloadStringView(payload), "input")
	if !input.Exists() {
		return false
	}
	hit := false
	checkItem := func(item gjson.Result) bool {
		var digests []string
		appendOpenAIEncryptedContentDigestsFromItem(item, &digests)
		appendOpenAIEncryptedAgentItemDigestsFromItem(item, &digests)
		for _, digest := range digests {
			if _, matched := invalid[digest]; matched {
				return true
			}
		}
		return false
	}
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			if checkItem(item) {
				hit = true
				return false
			}
			return true
		})
		return hit
	}
	if input.IsObject() {
		return checkItem(input)
	}
	return false
}

// stripOpenAIInvalidEncryptedContentFromReplayItems 对 replay 历史序列做同款
// 剥离。有改动时返回新头数组，命中项以重写后的新正文整体替换（遵守 replay
// 所有权不变式，原正文不被修改）；未命中时原样返回。
func stripOpenAIInvalidEncryptedContentFromReplayItems(items []json.RawMessage, invalid map[string]struct{}) ([]json.RawMessage, int) {
	if len(items) == 0 || len(invalid) == 0 {
		return items, 0
	}
	hit := false
	for _, item := range items {
		var digests []string
		parsed := gjson.Parse(openAIWSPayloadStringView(item))
		appendOpenAIEncryptedContentDigestsFromItem(parsed, &digests)
		appendOpenAIEncryptedAgentItemDigestsFromItem(parsed, &digests)
		for _, digest := range digests {
			if _, matched := invalid[digest]; matched {
				hit = true
				break
			}
		}
		if hit {
			break
		}
	}
	if !hit {
		return items, 0
	}
	stripped := 0
	next := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		var decoded map[string]any
		if err := decodeOpenAIJSONUseNumber(item, &decoded); err != nil {
			next = append(next, item)
			continue
		}
		wrapper := map[string]any{"input": decoded}
		changedCount := stripOpenAIInvalidEncryptedContentItems(wrapper, invalid)
		if changedCount == 0 {
			next = append(next, item)
			continue
		}
		stripped += changedCount
		nextItem, keep := wrapper["input"]
		if !keep {
			continue
		}
		rebuilt, err := marshalOpenAIUpstreamJSON(nextItem)
		if err != nil {
			next = append(next, item)
			stripped -= changedCount
			continue
		}
		next = append(next, json.RawMessage(rebuilt))
	}
	if stripped == 0 {
		return items, 0
	}
	return next, stripped
}

// stripOpenAIInvalidEncryptedContentRaw 是 raw JSON 版剥离：先零成本探测，命中
// 才解码改写重编码。返回改写后的 payload 与剥离项数；未命中时原样返回。
func stripOpenAIInvalidEncryptedContentRaw(payload []byte, invalid map[string]struct{}) ([]byte, int, error) {
	if !openAIRawPayloadHasInvalidEncryptedContent(payload, invalid) {
		return payload, 0, nil
	}
	var decoded map[string]any
	if err := decodeOpenAIJSONUseNumber(payload, &decoded); err != nil {
		return payload, 0, err
	}
	stripped := stripOpenAIInvalidEncryptedContentItems(decoded, invalid)
	if stripped == 0 {
		return payload, 0, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(decoded)
	if err != nil {
		return payload, 0, err
	}
	return rebuilt, stripped, nil
}

// markOpenAIWSInvalidEncryptedContentLineage 把本次被上游拒绝的密文摘要写入
// 会话 lineage。digests 须在剥离前收集。
func (s *OpenAIGatewayService) markOpenAIWSInvalidEncryptedContentLineage(groupID int64, sessionHash string, digests []string) {
	if s == nil || len(digests) == 0 || strings.TrimSpace(sessionHash) == "" {
		return
	}
	stateStore := s.getOpenAIWSStateStore()
	if stateStore == nil {
		return
	}
	stateStore.MarkSessionInvalidEncryptedContent(groupID, sessionHash, digests, s.openAIWSSessionStickyTTL())
}

// sessionInvalidEncryptedContentDigests 返回会话已知失效密文摘要；全局无记录
// 时（常态）零成本返回 nil。
func (s *OpenAIGatewayService) sessionInvalidEncryptedContentDigests(groupID int64, sessionHash string) map[string]struct{} {
	if s == nil || strings.TrimSpace(sessionHash) == "" {
		return nil
	}
	stateStore := s.getOpenAIWSStateStore()
	if stateStore == nil || !stateStore.HasAnySessionInvalidEncryptedContent() {
		return nil
	}
	return stateStore.GetSessionInvalidEncryptedContentDigests(groupID, sessionHash)
}

// openAIWSLineageSessionHashFromContext 取 lineage 会话键：优先 ingress 循环
// 写入的会话哈希（与读取侧同键），否则按请求体派生。
func (s *OpenAIGatewayService) openAIWSLineageSessionHashFromContext(c *gin.Context, body []byte) string {
	if c != nil {
		if fromCtx := strings.TrimSpace(c.GetString(openAIWSIngressSessionHashContextKey)); fromCtx != "" {
			return fromCtx
		}
	}
	return s.GenerateSessionHash(c, body)
}

// markOpenAIWSInvalidEncryptedContentLineageFromPayload 在上游以
// invalid_encrypted_content 拒绝 payload 时记录其密文摘要并输出观测日志。
func (s *OpenAIGatewayService) markOpenAIWSInvalidEncryptedContentLineageFromPayload(
	c *gin.Context,
	payload []byte,
	logKey string,
	accountID int64,
	turn int,
) {
	digests := collectOpenAIEncryptedContentDigestsRaw(payload)
	if len(digests) == 0 {
		return
	}
	s.markOpenAIWSInvalidEncryptedContentLineage(
		getOpenAIGroupIDFromContext(c),
		s.openAIWSLineageSessionHashFromContext(c, payload),
		digests,
	)
	logOpenAIWSModeInfo("%s account_id=%d turn=%d digests=%d", logKey, accountID, turn, len(digests))
}

// stripSessionInvalidEncryptedContentLogged 对 payload 执行会话失效密文剥离并
// 输出观测日志（logKey / logKey+"_skip"），返回（可能已替换的）payload 与剥离
// 项数；未命中或剥离失败时原样返回。
func (s *OpenAIGatewayService) stripSessionInvalidEncryptedContentLogged(
	payload []byte,
	invalid map[string]struct{},
	logKey string,
	accountID int64,
	turn int,
) ([]byte, int) {
	strippedPayload, strippedCount, stripErr := stripOpenAIInvalidEncryptedContentRaw(payload, invalid)
	if stripErr != nil {
		logOpenAIWSModeInfo(
			"%s_skip account_id=%d turn=%d reason=strip_error cause=%s",
			logKey,
			accountID,
			turn,
			truncateOpenAIWSLogValue(stripErr.Error(), openAIWSLogValueMaxLen),
		)
		return payload, 0
	}
	if strippedCount > 0 {
		logOpenAIWSModeInfo(
			"%s account_id=%d turn=%d stripped_items=%d",
			logKey,
			accountID,
			turn,
			strippedCount,
		)
	}
	return strippedPayload, strippedCount
}

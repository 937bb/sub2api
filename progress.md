## 2026-06-15 - Task: 完善 Sub Claude 使用方法
### What was done
- 读取当前项目中 Claude/Antigravity 网关路由、API Key 鉴权、Claude Code 客户端识别、上游转发、beta header 合并和分组调度相关源码。
- 对照两个 Claude Code CLI 源码镜像中的环境变量、认证头、base URL 和请求头行为，新增 Sub Claude 使用指南。
- 在 README 的 Antigravity/Claude Code 配置位置补充普通 Anthropic 路由示例，并增加完整指南入口。

### Testing
- 已执行 `Test-Path -LiteralPath docs\SUB_CLAUDE_USAGE.md`，确认新增文档存在。
- 已执行 `rg -n "Sub Claude Usage|ANTHROPIC_BASE_URL|ANTHROPIC_AUTH_TOKEN|/antigravity/v1/messages|allow_messages_dispatch|claude_code_only|gateway.go|api_key_auth.go" README.md docs\SUB_CLAUDE_USAGE.md`，确认关键配置、端点和文档入口可检索。
- 已执行源码引用路径检查，确认文档列出的 `backend/internal/server/routes/gateway.go`、`backend/internal/server/middleware/api_key_auth.go`、`backend/internal/server/middleware/middleware.go`、`backend/internal/handler/gateway_handler.go`、`backend/internal/handler/gateway_helper.go`、`backend/internal/service/claude_code_validator.go`、`backend/internal/service/gateway_service.go`、`backend/internal/pkg/claude/constants.go` 均存在。
- 未运行后端单元测试；本轮仅修改文档和 `.gitignore` 跟踪规则，不涉及运行逻辑。

### Notes
- `.gitignore`：新增 `docs/SUB_CLAUDE_USAGE.md` 例外，确保本轮正式文档可被版本跟踪；回滚可删除该例外行。
- `README.md`：补充 Sub Claude 完整指南链接，以及普通 Anthropic 与 Antigravity-only 两种 Claude Code 配置示例；回滚可还原 README 中 Antigravity Support 段落的本轮新增内容。
- `docs/SUB_CLAUDE_USAGE.md`：新增 Sub Claude 使用指南，覆盖环境变量、端点选择、后台配置、Claude Code 兼容说明、常见问题和源码依据；回滚可删除该文件。
- `progress.md`：追加本轮任务记录；回滚可删除本轮 `2026-06-15 - Task: 完善 Sub Claude 使用方法` 记录。

## 2026-06-15 - Task: 减少 Sub Claude 代理误杀
### What was done
- 对照 Claude Code CLI 源码确认普通对话认证请求使用 `ANTHROPIC_AUTH_TOKEN` 生成 `Authorization: Bearer ...`，并带 `x-app: cli`、`User-Agent: claude-cli/...`、`anthropic-version` 与 beta；文件/附件请求则走 `/v1/files` 或 `/api/oauth/file_upload` 的 OAuth/session-token 链路。
- 放宽 Sub2API 对 `/v1/messages` 的 Claude Code 客户端识别：在已满足 Claude CLI UA、必需 headers、且 system 中存在 `x-anthropic-billing-header` 与 `cc_entrypoint=cli` 时，不再强制要求 `metadata.user_id`。
- 保留普通 Claude Code identity prompt 的 metadata 校验，避免只靠相似 system prompt 就绕过 `claude_code_only` 分组限制。
- 补充使用文档中认证请求、文件请求边界和降低误杀后的识别规则说明。
### Testing
- 已执行 `C:\Go\bin\gofmt.exe -w backend\internal\service\claude_code_validator.go backend\internal\service\claude_code_validator_test.go backend\internal\handler\gateway_helper_hotpath_test.go`，完成 Go 格式化。
- 已执行 `C:\Go\bin\go.exe test ./internal/service -run 'TestClaudeCodeValidator_(BillingBlockRecognizedWithoutMetadata|IdentityPromptWithoutMetadataStillRejected|BillingBlockNonCLIEntrypointFallsThrough|BillingBlockStillRequiresClaudeCodeUA|MessagesPathFullValid)'`，通过。
- 已执行 `C:\Go\bin\go.exe test ./internal/handler -run 'TestSetClaudeCodeClientContext_(FastPathAndStrictPath|ReuseParsedRequest)'`，通过。
- 已执行 `rg -n "Claude Code file APIs|x-anthropic-billing-header|metadata.user_id|/api/oauth/file_upload|/v1/files" docs\SUB_CLAUDE_USAGE.md`，确认文档新增说明可检索。
### Notes
- `backend/internal/service/claude_code_validator.go`：新增 Claude Code billing attribution 识别分支，降低官方辅助 messages 请求因 metadata 缺失或形态变化被误杀的概率；回滚可移除 `hasClaudeCodeBillingBlock` 与 `Validate` 中对应提前通过分支。
- `backend/internal/service/claude_code_validator_test.go`：新增 billing block 无 metadata 可通过、identity prompt 无 metadata 仍拒绝的单元测试；回滚可删除本轮新增测试。
- `backend/internal/handler/gateway_helper_hotpath_test.go`：新增 handler 热路径和 ParsedRequest 复用路径的 billing block 识别测试；回滚可删除本轮新增 helper 与子测试。
- `docs/SUB_CLAUDE_USAGE.md`：补充 Claude Code 普通对话认证、文件/OAuth 请求边界和新识别规则；回滚可删除本轮新增段落。
- `progress.md`：追加本轮任务记录；回滚可删除本轮 `2026-06-15 - Task: 减少 Sub Claude 代理误杀` 记录。

## 2026-06-15 - Task: 隔离 Claude 代理指纹缓存污染
### What was done
- 将 Claude 账号指纹创建固定为服务端默认 Claude Code 指纹，不再从客户端 `User-Agent` 或 `X-Stainless-*` 读取初始值。
- 删除缓存命中后根据客户端 UA 版本回写指纹缓存的逻辑，保留 24 小时 TTL 续期，避免同一账号多窗口并发时互相污染缓存。
- 收紧 Claude 上游白名单，不再透传客户端 `user-agent`、`x-app` 和 `x-stainless-*` 指纹头。
- 为 Anthropic API-key passthrough 的 `/v1/messages` 与 `/v1/messages/count_tokens` 请求统一应用服务端 Claude Code 默认指纹，覆盖客户端指纹头。
- 更新 Sub Claude 使用文档，说明默认指纹统一行为和调试开关边界。
### Testing
- 已执行 `C:\Go\bin\gofmt.exe -w backend\internal\service\identity_service.go backend\internal\service\gateway_service.go backend\internal\service\identity_service_order_test.go backend\internal\service\gateway_anthropic_apikey_passthrough_test.go`，完成 Go 格式化。
- 已执行 `C:\Go\bin\go.exe test ./internal/service -run 'TestIdentityService_(CreateFingerprintFromHeadersUsesDefaultFingerprint|GetOrCreateFingerprintDoesNotMergeClientHeadersIntoCache)|TestGatewayAllowedHeadersExcludesClientFingerprintHeaders|TestGatewayService_AnthropicAPIKeyPassthrough_(BuildRequestAppliesGatewayFingerprint|BuildCountTokensRequestAppliesGatewayFingerprint|ForwardStreamPreservesBodyAndAuthReplacement|ForwardCountTokensPreservesBody)'`，通过。
- 已执行 `C:\Go\bin\go.exe test ./internal/service -run 'TestClaudeCodeValidator_(BillingBlockRecognizedWithoutMetadata|IdentityPromptWithoutMetadataStillRejected|BillingBlockNonCLIEntrypointFallsThrough|BillingBlockStillRequiresClaudeCodeUA|MessagesPathFullValid)|TestIdentityService_(CreateFingerprintFromHeadersUsesDefaultFingerprint|GetOrCreateFingerprintDoesNotMergeClientHeadersIntoCache)|TestGatewayAllowedHeadersExcludesClientFingerprintHeaders|TestGatewayService_AnthropicAPIKeyPassthrough_(BuildRequestAppliesGatewayFingerprint|BuildCountTokensRequestAppliesGatewayFingerprint|ForwardStreamPreservesBodyAndAuthReplacement|ForwardCountTokensPreservesBody)'`，通过。
### Notes
- `backend/internal/service/identity_service.go`：改为只使用服务端默认 Claude Code 指纹创建账号指纹，并移除客户端 UA 版本触发的缓存 merge/update；回滚可恢复客户端头参与指纹创建和 `isNewerVersion`/merge 分支。
- `backend/internal/service/gateway_service.go`：移除客户端指纹头白名单，并为 API-key passthrough messages/count_tokens 加入服务端默认指纹覆盖；回滚可恢复 `allowedHeaders` 中的指纹头并移除 `applyGatewayFingerprintForPassthrough` 调用。
- `backend/internal/service/identity_service_order_test.go`：新增默认指纹创建和缓存不被客户端头回写的回归测试；回滚可删除本轮新增测试和 stub 字段。
- `backend/internal/service/gateway_anthropic_apikey_passthrough_test.go`：新增白名单与两条 passthrough 指纹覆盖回归测试，并同步旧 passthrough 断言；回滚可恢复旧断言并删除本轮新增测试。
- `docs/SUB_CLAUDE_USAGE.md`：补充指纹统一默认行为、覆盖范围和调试开关说明；回滚可删除本轮新增段落。
- `progress.md`：追加本轮任务记录；回滚可删除本轮 `2026-06-15 - Task: 隔离 Claude 代理指纹缓存污染` 记录。
## 2026-06-15 - Task: 深挖 Claude Code CLI 请求链路并补齐代理请求 ID
### What was done
- 对照两个 Claude Code CLI 源码镜像确认：交互入口会设置 `CLAUDE_CODE_ENTRYPOINT=cli`，普通消息会带 Claude Code header 和 metadata，`count_tokens` 可能不带 metadata，文件上传走 `/v1/files` 或 `/api/oauth/file_upload` 的 OAuth/session-token 边界。
- 确认 CLI 只有在 `ANTHROPIC_BASE_URL` 是 Anthropic 官方一方 host 时才自动注入 `x-client-request-id`；当指向 Sub2API 代理时该头可能缺失。
- 在 Sub2API 转发 Claude 上游请求前补齐 `x-client-request-id`：缺失时生成 UUID，客户端已传时保持原值，覆盖 OAuth 默认头、OAuth mimic 头、Anthropic API-key passthrough messages/count_tokens 路径。
- 更新 Sub Claude 使用文档，说明代理部署下请求 ID 自动补齐行为。
### Testing
- 已执行 `C:\Go\bin\gofmt.exe -w backend\internal\service\gateway_service.go backend\internal\service\gateway_anthropic_apikey_passthrough_test.go`，完成 Go 格式化。
- 已执行 `C:\Go\bin\go.exe test ./internal/service -run 'TestGatewayService_AnthropicAPIKeyPassthrough_(BuildRequestAppliesGatewayFingerprint|BuildCountTokensRequestAppliesGatewayFingerprint|ForwardStreamPreservesBodyAndAuthReplacement|ForwardCountTokensPreservesBody)|TestGatewayService_AnthropicOAuth_NotAffectedByAPIKeyPassthroughToggle|TestGatewayAllowedHeadersExcludesClientFingerprintHeaders'`，通过。
- 已执行 `C:\Go\bin\go.exe test ./internal/service -run 'TestClaudeCodeValidator_(BillingBlockRecognizedWithoutMetadata|IdentityPromptWithoutMetadataStillRejected|BillingBlockNonCLIEntrypointFallsThrough|BillingBlockStillRequiresClaudeCodeUA|MessagesPathFullValid)|TestIdentityService_(CreateFingerprintFromHeadersUsesDefaultFingerprint|GetOrCreateFingerprintDoesNotMergeClientHeadersIntoCache)|TestGatewayAllowedHeadersExcludesClientFingerprintHeaders|TestGatewayService_AnthropicAPIKeyPassthrough_(BuildRequestAppliesGatewayFingerprint|BuildCountTokensRequestAppliesGatewayFingerprint|ForwardStreamPreservesBodyAndAuthReplacement|ForwardCountTokensPreservesBody)|TestGatewayService_AnthropicOAuth_NotAffectedByAPIKeyPassthroughToggle'`，通过。
- 已执行 `C:\Go\bin\go.exe test ./internal/handler -run 'TestSetClaudeCodeClientContext_(FastPathAndStrictPath|ReuseParsedRequest)'`，通过。
- 已执行 `git diff --check`，通过；仅提示 `.gitignore` 与 `README.md` 的既有 LF/CRLF 工作区转换 warning。
### Notes
- `backend/internal/service/gateway_service.go`：新增 `ensureClaudeClientRequestID` 并接入 OAuth 默认头、OAuth mimic 头和 API-key passthrough 指纹应用路径；回滚可移除该 helper 及三处调用。
- `backend/internal/service/gateway_anthropic_apikey_passthrough_test.go`：补充请求 ID 生成、客户端请求 ID 保留、OAuth 链路存在性断言；回滚可删除本轮新增断言和 `uuid` import。
- `docs/SUB_CLAUDE_USAGE.md`：补充代理部署下 `x-client-request-id` 自动补齐说明；回滚可删除该新增段落。
- `progress.md`：追加本轮任务记录；回滚可删除本轮 `2026-06-15 - Task: 深挖 Claude Code CLI 请求链路并补齐代理请求 ID` 记录。
## 2026-06-16 - Task: Align Sub Claude Code session and request fingerprint behavior
### What was done
- Aligned Claude Code identity rewriting with the CLI source by preserving distinct top-level CLI sessions for one upstream Claude account instead of collapsing them into an account-level masked session.
- Added upstream-side Claude Code request identity completion for standard Anthropic messages and count_tokens requests: default CLI headers for verified Claude Code API-key traffic, generated x-client-request-id when absent, and X-Claude-Code-Session-Id synchronized from final rewritten metadata when present.
- Updated Sub Claude usage docs for single-account multi-concurrency, session isolation, request ID completion, and max_sessions guidance.
### Testing
- Ran `C:\go\bin\gofmt.exe -w` on touched Go files.
- Ran `C:\go\bin\go.exe test ./internal/service -run "TestIdentityService_|TestBuildUpstreamRequest_APIKeyClaudeCodeAddsNativeHeadersAndSession|TestBuildCountTokensRequest_APIKeyClaudeCodeAddsNativeHeadersAndRequestID|TestGatewayAllowedHeadersExcludesClientFingerprintHeaders|TestGatewayService_AnthropicAPIKeyPassthrough_Build(Request|CountTokensRequest)AppliesGatewayFingerprint|TestClaudeCodeValidator_"` from `backend`; passed.
- Ran `C:\go\bin\go.exe test ./internal/service` from `backend`; passed.
- Ran `C:\go\bin\go.exe test ./internal/handler -run "Test.*ClaudeCode|Test.*GatewayHelper|Test.*Hotpath|Test.*Version"` from `backend`; passed.
- Ran `git diff --check`; passed with existing LF/CRLF warnings for `.gitignore` and `README.md`.
### Notes
- `backend/internal/service/identity_service.go`: kept the legacy `RewriteUserIDWithMasking` call site but made it preserve per-CLI-session identity through deterministic account/session rewriting; rollback by restoring the masked-session cache branch.
- `backend/internal/service/gateway_service.go`: added request ID completion, metadata-to-session-header synchronization, and verified-Claude-Code API-key fingerprint application for standard messages/count_tokens paths; rollback by removing `syncClaudeCodeSessionHeaderFromMetadata`, `applyClaudeCodeClientFingerprint`, and the new call sites.
- `backend/internal/service/identity_service_order_test.go`: updated and added identity tests proving session masking no longer collapses distinct CLI sessions; rollback by restoring the prior masked-session assertion and removing the new distinct-session test.
- `backend/internal/service/gateway_context_management_test.go`: added standard API-key messages/count_tokens tests for Claude Code native headers, request IDs, and session header sync; rollback by removing the two new tests and `uuid` import.
- `docs/SUB_CLAUDE_USAGE.md`: documented Claude Code multi-concurrency and session isolation behavior; rollback by deleting the new multi-concurrency/session paragraphs and `max_sessions` row.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-16` block.

## 2026-06-16 - Task: Reduce Claude Code proxy false positives in request identity and scheduling
### What was done
- Preserved valid `x-client-request-id` values from already-validated Claude Code clients instead of always regenerating them, while still generating an upstream UUID when the header is missing or malformed.
- Removed ambient browser/locale headers (`accept-language`, `sec-fetch-mode`) from Anthropic upstream passthrough so user API clients do not leak non-CLI environment traits into Claude Code-style requests.
- Added load-balancer tie-breaking for fresh Claude Code sessions: when eligible accounts have equal priority/load/LRU, the scheduler uses validated Claude Code metadata plus the Sub2API user id as a stable affinity seed, and prefers fewer active sessions for OAuth/setup-token accounts with `max_sessions`.
- Updated Sub Claude usage docs to state the request-id, body/timezone, header filtering, and account-pool scheduling behavior.
### Testing
- Ran `C:\go\bin\gofmt.exe -w backend\internal\service\gateway_service.go backend\internal\service\scheduler_layered_filter_test.go backend\internal\service\gateway_anthropic_apikey_passthrough_test.go backend\internal\service\gateway_context_management_test.go`.
- Ran `C:\go\bin\go.exe test ./internal/service -run "TestGatewayAllowedHeadersExcludesClientFingerprintHeaders|TestGatewayService_AnthropicAPIKeyPassthrough_(BuildRequestAppliesGatewayFingerprint|ClaudeCodePreservesRequestIDAndSyncsSession|BuildCountTokensRequestAppliesGatewayFingerprint)|TestBuildUpstreamRequest_APIKeyClaudeCodeAddsNativeHeadersAndSession|TestBuildCountTokensRequest_APIKeyClaudeCodeAddsNativeHeadersAndRequestID"` from `backend`; passed.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service -run "TestSelectByLRU|TestSelectByLRUWithAffinity|TestSchedulerAffinitySeed|TestFilterByMinActiveSessions|TestLayeredFilterIntegration"` from `backend`; passed.
- Ran `C:\go\bin\go.exe test ./internal/service` from `backend`; passed.
- Ran `C:\go\bin\go.exe test ./internal/handler -run "Test.*ClaudeCode|Test.*GatewayHelper|Test.*Hotpath|Test.*Version"` from `backend`; passed.
- Ran `git diff --check`; passed with existing LF/CRLF warnings for `.gitignore` and `README.md`.
### Notes
- `backend/internal/service/gateway_service.go`: tightened Anthropic header passthrough, preserved validated Claude Code request IDs, and added stable affinity plus active-session-aware tie-breaking in load selection; rollback by restoring the removed whitelist entries and removing `ensureClaudeClientRequestIDFromClient`, `filterByMinActiveSessions`, `selectByLRUWithAffinity`, `schedulerAffinitySeed`, and their call sites.
- `backend/internal/service/gateway_anthropic_apikey_passthrough_test.go`: updated passthrough tests for request-id preservation and browser/locale header filtering; rollback by restoring the previous request-id regeneration expectations and removing the new header assertions.
- `backend/internal/service/gateway_context_management_test.go`: updated Claude Code API-key request tests to expect valid client request IDs to be preserved; rollback by restoring the previous `NotEqual` request-id assertions.
- `backend/internal/service/scheduler_layered_filter_test.go`: added unit-tagged scheduler tests for stable affinity and active-session filtering; rollback by deleting the new test cases and helper stub.
- `docs/SUB_CLAUDE_USAGE.md`: documented that Sub2API does not rewrite body timezone/local date content, does not forward ambient browser/locale headers, and uses stable affinity/session counts for account-pool tie-breaking; rollback by deleting the new paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-16 - Task: Reduce Claude Code proxy false positives in request identity and scheduling` block.

## 2026-06-16 - Task: Review official Sub2API Claude updates and preserve safe rate-limit behavior
### What was done
- Reviewed official Sub2API main at `4a5665da5b2c6b83c4597844ea6e573746c821b1` against the current local base and separated safe account-pool stability changes from Claude request impersonation/template-injection changes.
- Kept the previous global Sub2API timezone default rollback intact: no global `TZ=America/New_York` runtime or deploy default remains in the current diff.
- Ported the safe Anthropic 5h/7d rate-limit window handling so official upstream reset headers take precedence over local temporary-unschedulable 429 rules.
- Preserved local session-window bookkeeping for Anthropic 5h rejected windows so scheduling can still see the account's Claude window state.
### Testing
- Ran `C:\go\bin\gofmt.exe -w backend\internal\service\ratelimit_service.go backend\internal\service\ratelimit_service_anthropic_window_limit_test.go`.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service -run "TestHandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule"` from `backend`; passed.
- Ran `C:\go\bin\go.exe test ./internal/service` from `backend`; passed.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service -run "TestHandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule|TestSelectByLRU|TestSelectByLRUWithAffinity|TestSchedulerAffinitySeed|TestFilterByMinActiveSessions|TestLayeredFilterIntegration"` from `backend`; passed.
- Ran `git diff --check`; passed with existing LF/CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
- Ran `rg -n "TZ=America/New_York|Sub2API defaults to the Eastern|Set Sub2API default timezone|default.*America/New_York|timezone-like values|local date" backend deploy docs progress.md README.md`; confirmed no global Eastern timezone default remains, and only the documented no-body-timezone-rewrite note matches.
### Notes
- `backend/internal/service/ratelimit_service.go`: added Anthropic official 5h/7d window preemption before local temp-unsched rules and retained 5h session-window updates; rollback by removing `persistAnthropicExhaustedWindowLimit`, its helper functions, and the new call before `tryTempUnschedulable`.
- `backend/internal/service/ratelimit_service_anthropic_window_limit_test.go`: added a unit regression test proving a local 10-minute 429 temp rule cannot shorten an official Anthropic 5h reset; rollback by deleting this test file.
- `docs/SUB_CLAUDE_USAGE.md`: documented that official Anthropic rate-limit windows are honored before local temporary scheduling rules; rollback by deleting the new rate-limit paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-16 - Task: Review official Sub2API Claude updates and preserve safe rate-limit behavior` block.

## 2026-06-17 - Task: 将 Sub Claude mimic 请求日期限定为美东
### What was done
- 将代理为非 Claude Code 客户端生成的 Claude OAuth/setup-token mimic system 扩展块追加 Claude Code 风格的当前日期，并用 `America/New_York` 计算日期。
- 保持 Sub2API 进程全局时区不变，也不改写真实 Claude Code 客户端提交的请求体。
- 按官方 Sub2API 小修同步 `max_tokens=1 + haiku` 探测拦截，使流式和非流式探测都走本地 mock 响应，减少无意义上游请求。
- 更新 Sub Claude 使用文档，说明美东日期只作用于代理生成的 mimic system block。
### Testing
- Ran `C:\Go\bin\gofmt.exe -w backend\internal\service\gateway_service.go backend\internal\service\gateway_prompt_test.go backend\internal\handler\gateway_handler.go backend\internal\handler\gateway_handler_intercept_test.go`.
- Ran `C:\Go\bin\go.exe test ./internal/service -run "TestRewriteSystemForNonClaudeCode|TestClaudeMimicCurrentDateTextUsesEasternDate"` from `backend`; passed.
- Ran `C:\Go\bin\go.exe test ./internal/handler -run "TestDetectInterceptType|TestIsMaxTokensOneHaikuRequest|TestSendMockInterceptResponse"` from `backend`; passed.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/service -run "TestHandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule"` from `backend`; passed.
- Ran `git diff --check`; passed with existing LF/CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/service/gateway_service.go`: added Claude mimic date helpers and appended the Eastern-date context to the generated non-Claude-Code system expansion block; rollback by removing `claudeMimicDateLocationName`, the three date helper functions, the `time/tzdata` import, and restoring `rewriteSystemForNonClaudeCode` to use `claudeCodeSystemPromptExpansion` directly.
- `backend/internal/service/gateway_prompt_test.go`: updated mimic system-block assertions and added a regression test proving UTC early-morning time maps to the previous Eastern date; rollback by restoring the exact expansion assertion and deleting `TestClaudeMimicCurrentDateTextUsesEasternDate`.
- `backend/internal/handler/gateway_handler.go`: removed the stream restriction from `max_tokens=1 + haiku` Claude Code probe detection and intercept routing; rollback by restoring the `isStream` parameter and `!isStream` condition.
- `backend/internal/handler/gateway_handler_intercept_test.go`: updated intercept tests for the new signature and added coverage for haiku probe shape independent of stream flag; rollback by restoring the old call signature and deleting `TestIsMaxTokensOneHaikuRequest_DoesNotDependOnStreamShape`.
- `docs/SUB_CLAUDE_USAGE.md`: documented that Eastern-date injection is limited to generated Claude OAuth/setup-token mimic system blocks and does not rewrite real Claude Code request bodies; rollback by restoring the previous body/timezone paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: 将 Sub Claude mimic 请求日期限定为美东` block.
## 2026-06-17 - Task: Align Claude Code beta order, billing fingerprint, and mimic date shape
### What was done
- Aligned the Claude OAuth mimic beta token order with locally captured Claude Code traffic while preserving the same token set.
- Changed generated Claude mimic date text to keep the Eastern-date behavior but use the CLI-observed `YYYY/MM/DD` shape.
- Updated Claude billing `cc_version` fingerprint selection to follow JavaScript string-index semantics, including non-ASCII text and surrogate-pair behavior.
- Documented the date shape, beta order, and billing fingerprint alignment in the Sub Claude usage guide.
### Testing
- Ran `C:\Go\bin\gofmt.exe -w backend\internal\pkg\claude\constants.go backend\internal\service\gateway_billing_block.go backend\internal\service\claude_code_js_string.go backend\internal\service\gateway_billing_block_test.go backend\internal\service\gateway_beta_test.go backend\internal\service\gateway_prompt_test.go backend\internal\service\gateway_service.go backend\internal\service\gateway_anthropic_apikey_passthrough_test.go`.
- Ran `C:\Go\bin\go.exe test ./internal/service -run "TestComputeClaudeCodeFingerprintMatchesJSStringIndexing|TestSignBillingHeaderCCH|TestSyncBillingHeaderVersion|TestFullClaudeCodeMimicryBetas|TestMergeAnthropicBetaDropping|TestClaudeMimicCurrentDateTextUsesEasternDate|TestRewriteSystemForNonClaudeCode"` from `backend`; passed.
- Ran `C:\Go\bin\go.exe test ./internal/service` from `backend`; passed.
- Ran `C:\Go\bin\go.exe test ./internal/handler -run "TestDetectInterceptType|TestIsMaxTokensOneHaikuRequest|TestSetClaudeCodeClientContext|Test.*GatewayHelper"` from `backend`; passed.
- Ran `C:\Go\bin\go.exe test ./internal/pkg/claude` from `backend`; passed with no test files.
- Ran `git diff --check`; passed with existing LF/CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/pkg/claude/constants.go`: moved `context-management-2025-06-27` ahead of prompt-caching/effort in the OAuth mimic beta list; rollback by restoring the previous order.
- `backend/internal/service/gateway_billing_block.go`: switched billing fingerprint character selection from byte indexing to the shared JS string-index helper; rollback by restoring direct `firstText[i]` byte selection.
- `backend/internal/service/claude_code_js_string.go`: added the UTF-16/UTF-8 helper used to mimic JavaScript string indexing; rollback by deleting this file after restoring byte indexing.
- `backend/internal/service/gateway_billing_block_test.go`: added JS-reference fingerprint tests for ASCII, BMP Unicode, surrogate-pair, and short-text cases; rollback by deleting this test file.
- `backend/internal/service/gateway_beta_test.go`: made the beta test assert the full CLI-observed ordering; rollback by returning to membership-only assertions.
- `backend/internal/service/gateway_prompt_test.go`: updated the Eastern-date regression to expect `YYYY/MM/DD`; rollback by restoring `YYYY-MM-DD` expectations.
- `backend/internal/service/gateway_anthropic_apikey_passthrough_test.go`: updated the billing-system-block preservation test to accept the appended current-date context; rollback by restoring the exact expansion-text assertion.
- `backend/internal/service/gateway_service.go`: changed the Claude mimic date formatter to `YYYY/MM/DD`; rollback by restoring `Format("2006-01-02")`.
- `docs/SUB_CLAUDE_USAGE.md`: documented CLI-observed date shape, beta token order, and JS-index billing fingerprint behavior; rollback by deleting the new wording.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Align Claude Code beta order, billing fingerprint, and mimic date shape` block.

## 2026-06-17 - Task: Audit local and public Claude CLI alignment gaps
### What was done
- Compared the local Claude Code installations, the public `huangserva/claude-code-cli` source mirror, local captured 2.1.179 behavior, and the current Sub2API Claude gateway state.
- Added a Claude CLI alignment audit that separates already-covered traits from remaining differences and marks the high-risk changes that require explicit approval.
- Updated the Sub Claude usage guide to state that `sdk-cli` is an official non-interactive entrypoint discovered in the review, but is not yet enabled for billing-block fallback validation.
- Allowed the new audit document through the existing docs ignore rules so it is visible to Git.
- Tightened the audit guidance so future validation does not blanket-allow every public-source entrypoint; the immediate target remains the locally captured `sdk-cli` `/v1/messages` path.
### Testing
- Ran `Get-Content -LiteralPath docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md`; confirmed the audit document renders with the expected sections and evidence table.
- Ran `rg -n "CLAUDE_CODE_ENTRYPOINT = isNonInteractive \? 'sdk-cli' : 'cli'|cc_entrypoint=cli|files-api-2025-04-14,oauth-2025-04-20|FINGERPRINT_SALT|Today's date is" "C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src" backend docs -S`; confirmed the audit claims map to public CLI source and current Sub2API docs/code.
- Ran `rg -n "sdk-cli|CLAUDE_CLI_ALIGNMENT_AUDIT|files-api-2025-04-14,oauth-2025-04-20|JavaScript string-index|YYYY/MM/DD" docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md`; confirmed the new documentation references are present.
- Ran `git status --short -- .gitignore docs progress.md`; confirmed `docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md` is visible to Git after the docs whitelist update.
- Ran `git check-ignore -v docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md progress.md`; confirmed the audit document is matched by the new negation whitelist instead of remaining hidden by `docs/*`.
- Ran `git diff --check -- .gitignore docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md progress.md`; passed with the existing `.gitignore` LF/CRLF working-copy warning.
- Ran source reads for public `main.tsx`, `tools\REPLTool\constants.ts`, and `utils\embeddedTools.ts`; confirmed the public source contains multiple entrypoints, while local capture evidence only proves `sdk-cli` for the reviewed `/v1/messages` flow.
### Notes
- `.gitignore`: allowed `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md` through the existing docs ignore pattern; rollback by deleting the added whitelist line if the audit document is not kept.
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the local/public Claude CLI comparison and remaining implementation boundary; rollback by deleting this file.
- `docs/SUB_CLAUDE_USAGE.md`: documented the confirmed `sdk-cli` entrypoint gap and linked the audit; rollback by removing the new `sdk-cli` sentence from the `/v1/messages` compatibility paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Audit local and public Claude CLI alignment gaps` block.

## 2026-06-17 - Task: Refine Claude CLI local-source and files-route audit
### What was done
- Rechecked the local Claude Code installation and the public `huangserva/claude-code-cli` source mirror, then clarified that version equality is not a useful compatibility requirement because Claude Code can switch package/native versions.
- Documented that OS/arch headers are not the active problem; the important compatibility traits are UA family, entrypoint, auth shape, beta order, system attribution, session identity, and file-route behavior.
- Added a concrete `/v1/files` implementation boundary based on public CLI file helper behavior and current Sub2API routing/token code.
- Updated the Sub Claude usage guide to state clearly that `/v1/files` is not currently exposed by Sub2API and must be implemented as a separate OAuth-only transparent files proxy after approval.
### Testing
- Ran `rg -n "Version equality|Runtime OS/arch|Files API Implementation Boundary|GatewayService.GetAccessToken|setup-token|current Sub2API route table does not yet expose|oauth-2025-04-20|sdk-cli" docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md`; confirmed the new audit and usage-guide claims are present.
- Ran `rg -n "/v1/files|files-api-2025-04-14|AccountTypeSetupToken|func \(s \*GatewayService\) GetAccessToken|func \(s \*GatewayService\) getOAuthToken" backend\internal\service\gateway_service.go backend\internal\service\claude_token_provider.go backend\internal\service\claude_token_provider_test.go docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md`; confirmed the files-route note and setup-token token path match the inspected code.
- Ran `rg -n "/files" backend\internal\server\routes\gateway.go`; no matches, confirming the current gateway route table does not expose `/v1/files`.
- Ran `git diff --check -- docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md .gitignore`; passed with the existing `.gitignore` LF/CRLF working-copy warning.
- No Go tests were run because this task changed only documentation and progress records, not runtime code.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: clarified local/public CLI evidence, version/OS-arch interpretation, and the safe `/v1/files` proxy boundary; rollback by removing the new version/OS-arch paragraphs and `Files API Implementation Boundary` section.
- `docs/SUB_CLAUDE_USAGE.md`: clarified that `/v1/files` is not yet exposed and points to the audit for the pending proxy work; rollback by deleting the new final sentence in the file-API paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Refine Claude CLI local-source and files-route audit` block.

## 2026-06-17 - Task: Audit optional Claude CLI identity traits
### What was done
- Re-scanned the public Claude CLI source for optional identity traits that can appear outside the baseline local CLI path, including SDK suffixes, workload markers, remote-session headers, custom headers, additional-protection headers, and extra JSON metadata.
- Added an optional-traits matrix to the Claude CLI alignment audit so these traits are classified as tolerated, not globally synthesized, or pending a specific SDK/remote use case.
- Updated the Sub Claude usage guide to explain that SDK/workload/remote markers are official context-specific traits but are not baseline requirements for normal Claude account-pool traffic.
### Testing
- Ran `rg -n "Optional Official Traits Reviewed|agent-sdk|client-app|cc_workload|x-client-app|x-claude-remote|CLAUDE_CODE_EXTRA_METADATA|Extra JSON metadata|official context-specific traits" docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md`; confirmed the new audit matrix and usage-guide note are present.
- Ran `rg -n "CLAUDE_AGENT_SDK_VERSION|CLAUDE_AGENT_SDK_CLIENT_APP|cc_workload|workload/|x-client-app|x-claude-remote-container-id|x-claude-remote-session-id|x-anthropic-additional-protection|ANTHROPIC_CUSTOM_HEADERS|CLAUDE_CODE_EXTRA_METADATA" C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src\utils\http.ts C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src\constants\system.ts C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src\services\api\client.ts C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src\services\api\claude.ts -S`; confirmed each documented optional trait maps to public source evidence.
- Ran `rg -n "claudeCodeUAPattern|claudeCodeCLIEntrypointMarker|ParseMetadataUserID|jsonUserID|allowedHeaders|normalizeClaudeCodeUpstreamIdentity" backend\internal\service\claude_code_validator.go backend\internal\service\metadata_userid.go backend\internal\service\gateway_service.go`; confirmed current Sub2API recognition and forwarding behavior used for the tolerance decisions.
- Ran `git diff --check -- docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md progress.md .gitignore`; passed with the existing `.gitignore` LF/CRLF working-copy warning.
- No Go tests were run because this task changed only documentation and progress records, not runtime code.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the optional official trait matrix and the extra metadata rewrite gap; rollback by deleting the `Optional Official Traits Reviewed` section and the extra metadata row.
- `docs/SUB_CLAUDE_USAGE.md`: added the short operational note about SDK/workload/remote markers being context-specific; rollback by deleting that paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Audit optional Claude CLI identity traits` block.

## 2026-06-17 - Task: Capture local Claude CLI 2.1.112 request traits
### What was done
- Ran the currently installed local Claude Code CLI against a local mock endpoint with an isolated `CLAUDE_CONFIG_DIR` and captured the real `/v1/messages` request shape.
- Confirmed the local 2.1.112 `--print` path uses `User-Agent: claude-cli/2.1.112 (external, sdk-cli)` and billing attribution `cc_entrypoint=sdk-cli`, matching the public source rule for non-interactive mode.
- Documented that OS/arch headers are runtime fingerprint traits (`Windows/x64` in the local capture), not the current account-pool scheduling problem.
- Updated the audit and usage guide to separate normal message beta variability from generated OAuth mimic beta behavior.
### Testing
- Ran local `claude --print "hello capture"` with `ANTHROPIC_BASE_URL=http://127.0.0.1:63491`, `ANTHROPIC_AUTH_TOKEN` set to a fake capture token, and a temporary `CLAUDE_CONFIG_DIR`; the CLI completed successfully and returned the mock response.
- Inspected the captured mock request and confirmed `Authorization: Bearer ...`, `x-app: cli`, `X-Claude-Code-Session-Id`, JSON `metadata.user_id`, `x-anthropic-billing-header`, `anthropic-version`, and `anthropic-beta` were present.
- Ran `rg -n "Local 2.1.112 Live Capture|claude-cli/2.1.112 \\(external, sdk-cli\\)|cc_entrypoint=sdk-cli|Windows|x64|Message beta variability|2.1.112 and 2.1.179|normal CLI \`/v1/messages\` beta lists|prompt-caching-scope" docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md`; confirmed the new evidence and guidance are present.
- Ran `git diff --check -- docs\CLAUDE_CLI_ALIGNMENT_AUDIT.md docs\SUB_CLAUDE_USAGE.md .gitignore progress.md`; passed with the existing `.gitignore` LF/CRLF warning.
- No Go tests were run because this task changed only documentation and progress records, not runtime code.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the local 2.1.112 live capture section and updated the entrypoint/beta/OS-arch conclusions; rollback by removing the `Local 2.1.112 Live Capture` section and restoring the edited table rows.
- `docs/SUB_CLAUDE_USAGE.md`: clarified that `sdk-cli` is confirmed by both local captures and that beta lists vary by version/auth path; rollback by restoring the prior `/v1/messages` compatibility and beta-order paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Capture local Claude CLI 2.1.112 request traits` block.

## 2026-06-17 - Task: Verify local Claude CLI probe and auth-header variants
### What was done
- Verified that the local Claude CLI's preliminary `HEAD /` probe is tolerated when the endpoint returns `404`, so Sub2API does not need a special root `HEAD /` route for Claude Code compatibility.
- Captured a local `ANTHROPIC_API_KEY` run and confirmed the same Claude Code request traits are preserved while auth switches to `x-api-key`.
- Updated the audit and usage guide so `ANTHROPIC_AUTH_TOKEN` remains the recommended proxy setup, but `ANTHROPIC_API_KEY` is documented as an official Claude Code shape rather than a non-CLI anomaly.
### Testing
- Ran local `claude --print "hello head fail capture"` against a mock that returned `404` for `HEAD /`; the CLI still completed `/v1/messages?beta=true` successfully.
- Ran local `claude --print "hello api key capture"` with `ANTHROPIC_API_KEY` and a temporary `CLAUDE_CONFIG_DIR`; captured `x-api-key`, `User-Agent: claude-cli/2.1.112 (external, sdk-cli)`, `X-Claude-Code-Session-Id`, JSON `metadata.user_id`, and `cc_entrypoint=sdk-cli`.
- Ran source and doc searches for auth-header, entrypoint, and probe traits across the public CLI mirror, Sub2API docs, and gateway code.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: documented the tolerated `HEAD /` probe failure and the two official local auth-header shapes; rollback by removing the added probe/auth paragraphs and table row.
- `docs/SUB_CLAUDE_USAGE.md`: clarified the difference between recommended proxy auth and official CLI `ANTHROPIC_API_KEY` behavior; rollback by restoring the previous `ANTHROPIC_AUTH_TOKEN` paragraph.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Verify local Claude CLI probe and auth-header variants` block.

## 2026-06-17 - Task: Classify remaining Claude CLI alignment status
### What was done
- Rechecked billing attribution/CCH behavior against public source, local captures, and current Sub2API implementation.
- Documented that `cch=00000` is the default local JS-package behavior, while Sub2API's optional CCH signing is only a compatibility approximation and not the official native attestation path.
- Added a current alignment matrix that separates covered items, pending high-risk runtime changes, and no-change-needed observations.
### Testing
- Ran source searches for `cch`, `NATIVE_CLIENT_ATTESTATION`, `signBillingHeaderCCH`, `enable_cch_signing`, and billing attribution across public CLI source, Sub2API code, docs, and tests.
- Read `backend/internal/service/gateway_billing_header.go`, `backend/internal/service/gateway_billing_header_test.go`, and the relevant `gateway_service.go` call sites to confirm CCH signing is optional and occurs after body mutation when enabled.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added CCH classification and the current alignment status matrix; rollback by deleting the `Billing CCH placeholder` row and `Current Alignment Status` section.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Classify remaining Claude CLI alignment status` block.

## 2026-06-17 - Task: Verify bundled CLI entrypoint rewrite behavior
### What was done
- Compared the public `main.tsx` entrypoint initialization with the local bundled 2.1.112 `cli.js` implementation.
- Found that the local bundle rewrites an existing `CLAUDE_CODE_ENTRYPOINT=cli` to `sdk-cli` when the run is non-interactive, while the public mirror currently returns early when the env var is already set.
- Confirmed with a live local capture that `CLAUDE_CODE_ENTRYPOINT=cli` plus `claude --print` still sends `User-Agent: claude-cli/2.1.112 (external, sdk-cli)` and `cc_entrypoint=sdk-cli`.
- Updated the audit to classify `sdk-cli` compatibility as a current packaged-CLI requirement, not just a public-source possibility.
### Testing
- Ran local `claude --print "hello entrypoint override"` with `CLAUDE_CODE_ENTRYPOINT=cli`, `ANTHROPIC_AUTH_TOKEN`, a mock `ANTHROPIC_BASE_URL`, and a temporary `CLAUDE_CONFIG_DIR`; the CLI completed successfully and the captured `/v1/messages` request used `sdk-cli`.
- Read local bundled `cli.js` around `function DH5` and public `main.tsx` around `initializeEntrypoint` to confirm the implementation difference.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the local-package entrypoint rewrite finding and strengthened the `sdk-cli` pending-action evidence; rollback by removing the new local-package difference paragraph and restoring the edited table rows.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Verify bundled CLI entrypoint rewrite behavior` block.

## 2026-06-17 - Task: Complete local and public Claude CLI trait cross-check
### What was done
- Rechecked the public Claude CLI mirror after fetch and confirmed the cached `origin/master` source is still at `290fdc9`.
- Compared the installed local bundled `cli.js` with public source for UA formatting, `sdk-cli` entrypoint rewriting, `x-app` values, billing attribution, auth headers, and Files API beta/path behavior.
- Updated the audit and usage guide to classify `sdk-cli`, `cli-bg`, and Files API as official context-specific traits while keeping runtime changes behind explicit approval.
### Testing
- Ran `where.exe claude` and `claude --version`; confirmed the active local CLI is the Volta package reporting `2.1.112 (Claude Code)`.
- Searched the installed bundled `cli.js` for `function DH5`, `claude-cli/`, `cc_entrypoint`, `x-anthropic-billing-header`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, `X-Claude-Code-Session-Id`, `cli-bg`, and `files-api-2025-04-14,oauth-2025-04-20`; confirmed the documented local-source traits exist.
- Read public source `utils/http.ts`, `constants/system.ts`, `services/api/client.ts`, and `services/api/filesApi.ts`; confirmed matching UA, billing, auth, session, and Files API shapes.
- Read Sub2API route/token/upstream code and confirmed `/v1/files` is not currently registered, while OAuth/setup-token access-token and upstream proxy/TLS primitives already exist for a later approved files proxy.
- Documentation validation and `git diff --check` were run after this append in the same task cycle.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the local bundled source cross-check and tightened the `x-app` and Files API status; rollback by deleting the `Local Bundled Source Cross-check` section and restoring the edited table rows.
- `docs/SUB_CLAUDE_USAGE.md`: clarified that `cli-bg` is a context marker and that the pending files proxy should target OAuth/setup-token accounts; rollback by restoring the edited file-API and Claude Code detection paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Complete local and public Claude CLI trait cross-check` block.

## 2026-06-17 - Task: Review official Sub2API Claude-related upstream updates
### What was done
- Compared the current branch with the configured Sub2API upstream `origin/main` at `4a5665d` and classified Claude-related upstream fixes into covered, missing, and cleanup-only buckets.
- Confirmed the local branch already includes Anthropic reset-window cooldown preservation and stream-shape-independent `max_tokens=1` Haiku probe interception.
- Confirmed the local branch does not yet include configurable Claude OAuth system prompt blocks, mapped-model-aware thinking filters, non-JSON 2xx failover, or SSE `event:error` body preservation.
- Added the upstream review matrix to the Claude CLI alignment audit and kept the runtime changes behind explicit approval because they touch Claude identity, request-body mutation, or response/failover handling.
### Testing
- Ran `git log --oneline --decorate --max-count=30 origin/main` and inspected the relevant upstream commits.
- Ran `git show --stat` / targeted diffs for `8ce7b9a8f`, `6baf00d78`, `ab9987b2e`, `6c7203d83`, `f6e0ebc6`, and `b256f9114`.
- Ran targeted `rg` checks for `SettingKeyEnableClaudeOAuthSystemPromptInjection`, `ResolveThinkingProtocol`, `FilterThinkingBlocksForRetry`, `invalidNonStreamingJSONFailoverError`, `sseStreamErrorEventError`, Anthropic reset-window persistence, Haiku probe interception, and current `cc_entrypoint` validation.
- Ran public and local Claude CLI source searches for UA formatting, `sdk-cli`, billing attribution, `x-app`, and Files API beta literals.
- Ran documentation validation and `git diff --check` after this append in the same task cycle.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the official Sub2API upstream update review and expanded the approved-next-patch list; rollback by deleting the `Official Sub2API Upstream Update Review` section and restoring the edited status/proposed-patch rows.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-17 - Task: Review official Sub2API Claude-related upstream updates` block.

## 2026-06-18 - Task: Recheck Claude CLI and Sub2API upstream evidence after resume
### What was done
- Rechecked the active local Claude Code CLI, public `huangserva/claude-code-cli` mirror, and configured Sub2API `origin/main` after the goal resumed.
- Confirmed the external evidence did not change: local CLI still reports `2.1.112`, public CLI mirror remains at `290fdc9`, and Sub2API upstream remains at `4a5665d`.
- Updated the audit to record the 2026-06-18 recheck and to explicitly include thinking filters, non-JSON/SSE failover, and configurable OAuth system blocks in the high-risk approval boundary.
### Testing
- Ran `where.exe claude` and `claude --version`; confirmed the active CLI is the Volta package reporting `2.1.112 (Claude Code)`.
- Ran `git -C C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src ls-remote origin refs/heads/master HEAD`; confirmed both refs resolve to `290fdc9`.
- Ran `git ls-remote origin refs/heads/main HEAD`; confirmed both refs resolve to `4a5665d`.
- Ran documentation searches for the new 2026-06-18 recheck and approval-boundary text, then ran `git diff --check` for the changed docs and progress files.
- No Go tests were run because this task only refreshed evidence and documentation; no runtime code changed in this task cycle.
### Notes
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: added the 2026-06-18 recheck note and expanded the safe implementation boundary; rollback by removing the recheck paragraph and restoring the boundary list.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-18 - Task: Recheck Claude CLI and Sub2API upstream evidence after resume` block.

## 2026-06-18 - Task: Implement approved Claude CLI alignment runtime fixes
### What was done
- Added official `sdk-cli` billing attribution recognition for Claude Code requests while keeping the existing Claude CLI user-agent, required-header, billing-prefix, and unsupported-entrypoint rejection boundaries.
- Ported the approved upstream-response hygiene fixes: non-streaming 2xx responses with non-JSON bodies now produce a 502 failover error with upstream evidence, and SSE `event:error` frames now preserve the raw event data in failover and ops context.
- Added mapped-model-aware thinking protocol guards so Anthropic-strict models still get signature cleanup, while DeepSeek/Kimi/Moonshot/GLM/MiniMax M/Qwen thinking and unknown mapped upstreams keep thinking histories unchanged.
- Updated Claude usage and alignment docs so the implemented fixes are no longer listed as pending approval; remaining separate work is configurable OAuth system prompt blocks and `/v1/files` proxying.
### Testing
- Ran `C:\go\bin\gofmt.exe -w backend/internal/service/claude_code_validator.go backend/internal/service/claude_code_validator_test.go backend/internal/service/gateway_service.go backend/internal/service/gateway_request.go backend/internal/service/gemini_messages_compat_service.go backend/internal/service/thinking_protocol.go backend/internal/service/thinking_protocol_test.go backend/internal/service/thinking_protocol_filter_integration_test.go backend/internal/service/gateway_non_streaming_response_test.go backend/internal/service/gateway_sse_error_test.go`; formatting completed successfully.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service -run "TestClaudeCodeValidator_(BillingBlockRecognizedWithoutMetadata|BillingBlockSDKCLIRecognizedWithoutMetadata|BillingBlockNonCLIEntrypointFallsThrough)|TestResolveThinkingProtocol|TestThinkingProtocolGuards|TestFilterThinkingBlocks_(SkipsForPassbackRequired|SkipsForUnknownModel|StripsForAnthropicStrict)|TestFilterThinkingBlocksForRetry_SkipsForPassbackRequired|TestFilterSignatureSensitiveBlocksForRetry_SkipsForPassbackRequired|TestHandleNonStreamingResponse_(NonJSON2xxTriggersFailover|ValidJSONUnchanged)|TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_NonJSON2xxTriggersFailover|TestHandleStreamingResponse_SSEErrorEvent_(ReturnsTypedErrorWithRawData|NonJSONDataLine)"`; passed.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service ./internal/handler -run "Test(ClaudeCodeValidator_|ResolveThinkingProtocol|ThinkingProtocolGuards|FilterThinkingBlocks|FilterThinkingBlocksForRetry|FilterSignatureSensitiveBlocksForRetry|HandleNonStreamingResponse|HandleNonStreamingResponseAnthropicAPIKeyPassthrough|HandleStreamingResponse_SSEErrorEvent|SignBillingHeaderCCH|SyncBillingHeaderVersion|ComputeClaudeCodeFingerprintMatchesJSStringIndexing|HandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule|IsMaxTokensOneHaikuRequest|DetectInterceptType|SendMockInterceptResponse)"`; passed.
- Ran `git diff --check`; passed with existing LF-to-CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/service/claude_code_validator.go`: accepted `cc_entrypoint=sdk-cli` in Claude Code billing-block fallback; rollback by removing `claudeCodeSDKCLIEntrypointMarker`, the marker list/helper, and restoring direct `cc_entrypoint=cli` checks.
- `backend/internal/service/claude_code_validator_test.go`: added `sdk-cli` fallback coverage; rollback by deleting `TestClaudeCodeValidator_BillingBlockSDKCLIRecognizedWithoutMetadata`.
- `backend/internal/service/gateway_request.go`: added optional mapped-model guards to thinking filters; rollback by removing the optional parameter checks and restoring the original signatures.
- `backend/internal/service/gateway_service.go`: added non-JSON 2xx failover handling, SSE event-error body preservation, and mapped-model signature-rectifier gating; rollback by reverting the helper/type additions and the updated call sites.
- `backend/internal/service/gemini_messages_compat_service.go`: passed mapped model into retry filters; rollback by restoring the two original filter calls without model arguments.
- `backend/internal/service/thinking_protocol.go`: added the thinking protocol classifier; rollback by deleting this file and the mapped-model guards that call it.
- `backend/internal/service/thinking_protocol_test.go`, `backend/internal/service/thinking_protocol_filter_integration_test.go`, `backend/internal/service/gateway_non_streaming_response_test.go`, `backend/internal/service/gateway_sse_error_test.go`: added regression coverage for the approved fixes; rollback by deleting these files.
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`: marked the implemented approved fixes as covered and narrowed the remaining patch list; rollback by restoring the previous pending-status rows.
- `docs/SUB_CLAUDE_USAGE.md`: documented `sdk-cli` billing fallback and mapped-model thinking preservation; rollback by restoring the edited Claude Code compatibility paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-18 - Task: Implement approved Claude CLI alignment runtime fixes` block.

## 2026-06-18 - Task: Implement approved Claude OAuth prompt configuration and Files API proxy
### What was done
- Ported configurable Claude OAuth system prompt block settings from the reviewed official Sub2API update while preserving the built-in three-block default for generated Claude OAuth mimic traffic.
- Added admin/API settings for `enable_claude_oauth_system_prompt_injection`, `claude_oauth_system_prompt`, and `claude_oauth_system_prompt_blocks`, including fallback behavior when block JSON is invalid.
- Added a narrow `/v1/files` proxy for Claude Code file list/upload/metadata/delete/content routes that selects only Anthropic OAuth/setup-token accounts and forwards raw bodies with the official files beta header.
- Updated Claude usage and alignment docs to mark system prompt block configuration and `/v1/files` as implemented, while documenting that `/api/oauth/file_upload` remains outside this patch.
### Testing
- Ran `C:\go\bin\gofmt.exe -w` on the touched Go files for settings, gateway prompt handling, Files proxy, handler, routes, and related tests.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service ./internal/handler ./internal/server/routes -run "Test(GatewayService_ForwardClaudeFiles_UsesOAuthHeadersAndPath|BuildClaudeFilesURL_CustomBaseAvoidsDoubleV1|BuildClaudeFilesURL_CustomBaseEnabledRequiresURL|ForwardClaudeFilesRejectsAPIKeyAccount|RewriteSystemForNonClaudeCode|BuildClaudeOAuthSystemPromptBlocks|ClaudeMimicCurrentDateText|SettingService_UpdateSettings_ClaudeOAuthSystemPromptSettings|SettingService_ParseSettings_ClaudeOAuthSystemPromptDefaultsEnabled|SettingService_GetClaudeOAuthSystemPromptInjectionSettings|HandleNonStreamingResponse|HandleStreamingResponse_SSEErrorEvent|ClaudeCodeValidator_|IsMaxTokensOneHaikuRequest|DetectInterceptType|SendMockInterceptResponse)"`; passed.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildUpstreamRequest_APIKeyClaudeCodeAddsNativeHeadersAndSession|Test(GatewayService_ForwardClaudeFiles_UsesOAuthHeadersAndPath|BuildClaudeFilesURL_CustomBaseAvoidsDoubleV1|BuildClaudeFilesURL_CustomBaseEnabledRequiresURL|ForwardClaudeFilesRejectsAPIKeyAccount|RewriteSystemForNonClaudeCode|BuildClaudeOAuthSystemPromptBlocks|ClaudeMimicCurrentDateText|SettingService_UpdateSettings_ClaudeOAuthSystemPromptSettings|SettingService_ParseSettings_ClaudeOAuthSystemPromptDefaultsEnabled|SettingService_GetClaudeOAuthSystemPromptInjectionSettings)"`; passed.
- Ran `C:\go\bin\go.exe test -tags unit ./internal/service ./internal/handler ./internal/server/routes`; passed.
- Ran `git diff --check`; passed with existing LF-to-CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/service/domain_constants.go`: added Claude OAuth system prompt setting keys; rollback by removing the three new setting constants.
- `backend/internal/service/settings_view.go`, `backend/internal/service/setting_service.go`: carried the new settings through cache/view/default/update/read paths; rollback by removing the new fields, cache entries, defaults, and getter.
- `backend/internal/handler/dto/settings.go`, `backend/internal/handler/admin/setting_handler.go`: exposed the new settings in admin read/update responses and audit diffs; rollback by removing the new DTO/request fields and mapping/diff code.
- `backend/internal/service/gateway_billing_block.go`, `backend/internal/service/gateway_service.go`: made billing block text reusable and used configured/default Claude OAuth system prompt blocks during non-Claude-Code OAuth mimic rewriting; rollback by restoring static block construction and deleting the prompt-block helpers.
- `backend/internal/server/routes/gateway.go`, `backend/internal/handler/gateway_handler.go`: registered and handled `/v1/files` routes with Anthropic-only scheduling and OAuth/setup-token account filtering; rollback by deleting the files route registrations and `Files`/`selectClaudeFilesAccount` handler methods.
- `backend/internal/service/gateway_files.go`: added the transparent Claude Files API upstream proxy with OAuth token replacement, files beta header, custom base URL validation, TLS/profile reuse, response-header filtering, and session-window header sampling; rollback by deleting this file.
- `backend/internal/service/gateway_files_test.go`, `backend/internal/service/gateway_prompt_test.go`, `backend/internal/service/setting_service_update_test.go`, `backend/internal/service/gateway_context_management_test.go`: added or adjusted regression coverage for files proxying, prompt block templates, settings persistence, and Claude Code session/header alignment; rollback by deleting the new Files tests and restoring the edited assertions/new test cases.
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`, `docs/SUB_CLAUDE_USAGE.md`: documented the implemented settings and Files proxy behavior; rollback by restoring the prior pending-status sections and removing the new usage paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-18 - Task: Implement approved Claude OAuth prompt configuration and Files API proxy` block.

## 2026-06-18 - Task: Continue Claude Files and OAuth prompt alignment check
### What was done
- Rechecked the implemented Claude Files API proxy against the message scheduling path and fixed one gap: when a selected Anthropic OAuth/setup-token account is full but has a wait plan, `/v1/files` now waits for the account slot instead of failing immediately.
- Rechecked configurable Claude OAuth system prompt blocks and added a fallback for valid-but-empty block JSON so generated OAuth mimic requests keep the built-in Claude Code three-block shape.
- Tightened Files proxy coverage to prove inbound client fingerprint headers are replaced by the service-side Claude Code defaults before upstream forwarding.
- Updated the usage documentation to record the Files wait-plan behavior and the empty prompt-block fallback.
### Testing
- Ran `C:\Go\bin\gofmt.exe -w backend\internal\handler\gateway_handler.go backend\internal\service\gateway_service.go backend\internal\service\gateway_files_test.go backend\internal\service\gateway_prompt_test.go`.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/service -run "TestGatewayService_ForwardClaudeFiles|TestForwardClaudeFilesRejectsAPIKeyAccount|TestBuildClaudeFilesURL|TestRewriteSystemForNonClaudeCodeWithPromptBlocks|TestBuildClaudeOAuthSystemPromptBlocks"`; passed.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/handler ./internal/server/routes`; passed.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/service ./internal/handler ./internal/server/routes`; passed.
- Ran `git diff --check`; passed with existing LF-to-CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/handler/gateway_handler.go`: aligned `/v1/files` account-slot waiting with the normal message forwarding wait-plan path; rollback by removing the wait-plan acquisition block in `selectClaudeFilesAccount` and restoring the previous immediate capacity error.
- `backend/internal/service/gateway_service.go`: made empty configured OAuth prompt blocks fall back to the default configured prompt shape; rollback by restoring the single-error return in `rewriteSystemForNonClaudeCodeWithPromptBlocks`.
- `backend/internal/service/gateway_files_test.go`: added assertions that inbound fingerprint headers are not forwarded upstream; rollback by removing the added inbound header setup and assertions.
- `backend/internal/service/gateway_prompt_test.go`: added regression coverage for valid-but-empty OAuth prompt block configuration; rollback by deleting `TestRewriteSystemForNonClaudeCodeWithPromptBlocks_EmptyConfigFallsBackToDefaults`.
- `docs/SUB_CLAUDE_USAGE.md`: documented the Files wait-plan behavior and prompt-block fallback behavior; rollback by removing the two added sentences.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-18 - Task: Continue Claude Files and OAuth prompt alignment check` block.

## 2026-06-18 - Task: Align generated Claude mimic fingerprint with local CLI capture
### What was done
- Re-captured the local Claude Code CLI against a mock endpoint and confirmed the actual network request currently uses `claude-cli/2.1.181 (external, sdk-cli)` with Stainless package `0.94.0`, runtime `v24.3.0`, billing attribution without `cch`, and current-date context as the first user `<system-reminder>`.
- Updated generated Claude mimic defaults to match the current network fingerprint while preserving real validated Claude Code clients' inbound official UA/Stainless values.
- Changed generated billing attribution to omit the legacy `cch` field by default and kept optional CCH signing only for bodies that already contain a `cch=00000` placeholder.
- Moved generated current-date mimic context from the system expansion block into a first user `<system-reminder>` before billing fingerprint calculation.
- Updated the Claude alignment audit and usage guide to record the latest capture, the version-command versus network-fingerprint mismatch, and the remaining conservative beta-token stance.
### Testing
- Ran a local Node mock capture with `claude --print` and `ANTHROPIC_AUTH_TOKEN`; captured `POST /v1/messages?beta=true` using `claude-cli/2.1.181 (external, sdk-cli)`, `x-stainless-package-version: 0.94.0`, `x-stainless-runtime-version: v24.3.0`, billing text `cc_entrypoint=sdk-cli` without `cch`, and current-date context in `messages[0]`.
- Ran the same local mock capture with `ANTHROPIC_API_KEY`; confirmed the same fingerprint shape with `x-api-key` auth.
- Ran `C:\Go\bin\gofmt.exe -w backend/internal/service/gateway_service.go backend/internal/service/gateway_prompt_test.go backend/internal/service/gateway_anthropic_apikey_passthrough_test.go backend/internal/service/gateway_billing_block.go backend/internal/service/gateway_billing_header_test.go backend/internal/service/gateway_billing_block_test.go backend/internal/pkg/claude/constants.go`.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/service -run "Test(RewriteSystemForNonClaudeCode|BuildClaudeOAuthSystemPromptBlocks|ClaudeMimicCurrentDateText|GatewayService_AnthropicOAuth_ForwardPreservesBillingHeaderSystemBlock|BuildBillingAttributionBlockText|SyncBillingHeaderVersion)"`; passed.
- Ran `C:\Go\bin\go.exe test -tags unit ./internal/service ./internal/handler ./internal/server/routes`; passed.
- Ran `git diff --check`; passed with existing LF-to-CRLF working-copy warnings for `.gitignore`, `README.md`, `deploy/.env.example`, and `deploy/README.md`.
### Notes
- `backend/internal/pkg/claude/constants.go`: updated the default generated Claude Code fingerprint to the latest local network capture; rollback by restoring the previous `CLICurrentVersion` and `DefaultHeaders` values.
- `backend/internal/service/gateway_billing_block.go`: generated billing text now follows the current no-`cch` SDK-CLI shape; rollback by restoring the previous `cch=00000` format string.
- `backend/internal/service/gateway_billing_header.go`: keeps billing `cc_entrypoint` synchronized with the actual upstream UA; rollback by removing `ccEntrypointInBillingRe` and the entrypoint replacement.
- `backend/internal/service/gateway_service.go`: preserves validated Claude Code client fingerprints and inserts generated current-date reminder before building mimic system blocks; rollback by restoring unconditional default fingerprint application and removing the reminder helpers/call.
- `backend/internal/service/gateway_prompt_test.go`, `backend/internal/service/gateway_anthropic_apikey_passthrough_test.go`, `backend/internal/service/gateway_billing_block_test.go`, `backend/internal/service/gateway_billing_header_test.go`, `backend/internal/service/gateway_context_management_test.go`: updated regression coverage for preserved inbound fingerprints, SDK-CLI billing, no default CCH, and current-date reminder placement; rollback by restoring prior expected system/message/billing assertions.
- `docs/CLAUDE_CLI_ALIGNMENT_AUDIT.md`, `docs/SUB_CLAUDE_USAGE.md`: updated operational documentation for the latest CLI capture and current implementation behavior; rollback by restoring the previous capture/status paragraphs.
- `progress.md`: appended this task record; rollback by deleting this `2026-06-18 - Task: Align generated Claude mimic fingerprint with local CLI capture` block.

# 937sub2b SIWC 接入说明

本功能基于定制版 `d07453b28`，为 OpenAI 账号增加独立的 SIWC 授权与请求通道，无需数据库迁移。C → B 的 `127.0.0.1:6064`、现有客户 API key、分组、订阅和计费配置保持原值。部署版本与验证结果以对应发布记录为准。

## 参考与证据范围

协议参考来自用户提供的 `FenJue-Windows` Python 源码和 `siwc.so` 静态分析。前者提供独立授权与公共 API 路由；后者提供自动刷新与 Responses 字段适配的参考。没有加载或执行原始 so。

协议兼容性仍需真实 SIWC 授权验证。2026-10-08 使用指定线上账号的现有 Codex OAuth token 调用公共 Responses，收到 HTTP 401，错误为 `Missing scopes: api.responses.write`；token 尚未过期，未产生模型回答。该结果证明这份旧授权不能直接复用，不能证明 SIWC 新授权、模型调用、质量或上游扣费已经验证通过。上游是否开放、账号是否取得共享权限，必须以实际授权和模型目录为准。

## 管理页使用

1. 账号管理 → **添加账号** → **OpenAI** → **OAuth** → **下一步**，在「授权方式」中选择 **SIWC**。直接在同一个添加弹窗完成授权，沿用第一步的名称、代理、OpenAI 分组和并发数；账号列表顶部不再提供独立 SIWC 按钮。
2. 选择服务端代理、并发和明确需要绑定的 OpenAI 分组；不选分组则保存为未绑定账号，忽略普通 OAuth 全局自动分组策略。
3. 生成并打开授权链接，由用户在 OpenAI 域名下登录并同意授权。
4. 浏览器跳转到 `http://127.0.0.1:1455/auth/callback?...` 后，将完整地址粘贴回管理页。没有本机回调监听器时浏览器显示无法连接，这是手动回填方式的预期行为。
5. 在十分钟内保存。服务端校验 state、PKCE、ID token 签名、issuer、audience、azp、nonce、subject 和共享 scope，再读取可用模型并原子创建账号及分组关系。

重新授权使用账号菜单中的原入口，自动切换到 SIWC 流程，沿用原 host ID、client ID 和服务端代理，并要求 subject 一致。仅更新授权和可用模型目录；账号 ID、名称、倍率、分组、自定义模型映射保持原值。原本停用或错误状态的账号不会被自动启用，请检查后使用已有恢复状态功能。

授权会话暂存在 B 端内存，重启或超过十分钟需重新生成链接。多副本部署时，生成链接与回填必须路由至同一进程。运行目录 `siwc/host-id` 持久化稳定 host UUID，首次可沿用浏览器此前保存的 UUID；已建账号重新授权始终保留其原 host ID。首次授权在消费 code 前保存不含 token 的 issued client 注册记录；24 小时内点击「重新开始」会携带原会话标识，沿用该 client 并生成新的 state、nonce 和 PKCE。

AT、RT 和 ID token 不放入浏览器持久化存储。重新授权链接只发往官方 authorize 端点，使用已保存的 ID token hint 和 email hint，仍校验新 ID token 的身份。授权 URL 响应设置 no-store，审计记录隐藏完整授权和回调地址。

## 请求与计费

账号仍为 OpenAI OAuth 类型，以 `credentials.auth_mode=siwc` 区分：

账号列表中的 `OpenAI · SIWC` 与后端分流使用授权标识。已通过 SIWC 授权流程保存的 OpenAI OAuth 账号使用公共 Responses；原 OAuth 账号继续使用 `https://chatgpt.com/backend-api/codex/responses`。兼容旧导入中的 `extra.auth_protocol=siwc` 和 `oaiapp_` client ID：这些账号不会再落入 Codex 路径，但必须修复授权元数据后才能请求。名称、备注、客户端请求头不参与判断；API Key 账号保持原有官方或自定义上游。普通编辑不能把原 OAuth 直接改成 SIWC，仅添加标签的新建请求会被拒绝。

管理员可调用 `POST /api/v1/admin/openai/siwc/accounts/:id/repair` 修复旧 SIWC 导入。服务端先验证原 access token 的 JWKS 签名、issuer、resource audience、client ID、subject、scope 和时间字段，再以完整旧凭据及代理为条件原子补齐元数据。过期 AT 仅用于证明历史身份，修复不延长其有效期，也不旋转 RT；随后通过已有刷新入口更新，再获取模型目录。账号状态、分组、价格与映射不自动改动。未保存的原 host ID 不会被伪造；后续同一 subject/client 的重新授权可以绑定新会话 host。

2026-10-08 对指定 SIWC 账号的目录逐项直连实测，以下 7 个模型均返回 HTTP 200、完整 `response.completed` 及 `OK`：`gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-6-astra`、`gpt-6-luna`、`gpt-6-sol`、`gpt-6.1-sol`，单次约 1.5–2.6 秒。这是该账号当时的授权范围，不代表所有 OpenAI 模型或所有账号；`gpt-5.5`、`codex-auto-review` 不在该目录。目录获取失败时管理页不回退展示普通 Codex 的默认全模型。

| 路径 | SIWC 行为 |
| --- | --- |
| Responses | 固定 POST `https://api.openai.com/v1/responses` |
| Chat Completions / Messages | 经过现有兼容转换，再使用上述 Responses 路径 |
| token count | 使用已有本地估算，不向未经验证的上游端点发请求 |
| Compact、Live、AlphaSearch、Codex WebSocket、BPS、Prism、Shadow | 不用于 SIWC |
| Codex 额度查询、隐私设置、token guard、自动重登 | 排除 SIWC，避免 token 发错端点 |

保留 `developer` 与原始指令，将 `system` 就地转换为 `developer`；保留完整消息历史和推理密文，function/custom tools 转入具有 `role=developer` 的 additional_tools。无状态历史消息移除不兼容的非 msg_ 消息 ID；工具 call_id、工具结果关联和 reasoning ID 不改。强制上游 `store=false`、SSE 流式。非流式客户由现有兼容层汇总响应。非空 `previous_response_id` / `conversation`、item reference 和 compaction trigger 明确拒绝，不静默丢失状态。

结构化 `subscription_sharing_usage_limit_exceeded` 按 429 记录和返回；该 SIWC 授权暂停派发 60 秒，避免同一失败请求反复重试。60 秒是本地冷却，不是官方额度 reset；不据此声称整个套餐耗尽。`subscription_sharing_usage_unavailable` 按 503 处理。已输出内容的流保留终止错误，不重新播放请求。应用限额仍需在 ChatGPT Settings → Usage 核对。

不引入草稿、评价、重写等额外模型调用，也不缓冲完整回答后伪装流式。请求继续经过现有鉴权、调度、并发准入、模型映射、usage 解析与账务链；测试验证 input/output/cache token 进入原有结果结构。公共 API 返回的实际服务档位可以降低计费档位，普通 Codex 原行为保持不变。

实际出网时再次检查模型属于该授权的目录，并严格限制推理 URL。客户 Cookie、Authorization 和 Codex 身份头不会透传。SIWC 代理不可用时直接失败，不静默切换出口。

## Prompt cache

SIWC Responses 保留客户提供的 `prompt_cache_options` 和内容中的 `prompt_cache_breakpoint`，不再因为客户端不是 Codex CLI 就过滤缓存选项。仍移除官方明确不支持的旧字段 `prompt_cache_retention`；不自动添加 TTL、显式断点或缓存选项。

Chat Completions / Messages 兼容转换得到的会话缓存键在 SIWC 出网前写入 JSON 的 `prompt_cache_key`，不转成 Codex 的 `session_id` / `conversation_id` 请求头。原生 Responses 或 Responses 形状兼容请求已有的非空 body key 优先于转换链路的备用 key。所有 SIWC 路径统一按下游 API key ID、SIWC subject 和 client ID 派生稳定标识；同一授权刷新 token 不改变标识，不同租户或授权使用不同标识。仅修改新建出站 body，重试不在原始请求上重复哈希。

这会一次性改变此前原样透传的 SIWC 缓存标签，部署后的首次请求可能需要预热。缓存命中由上游决定：输入前缀、模型、工具定义、断点、有效期和最小可缓存长度仍需满足官方要求。现代模型的 `prompt_cache_key` 用于缓存计量分隔，并非提升路由命中的必需字段；补齐该字段不能保证命中率提升。已缓存 token 继续从上游 usage 读取，不伪造命中或修改账务规则。

## 身份与网络指纹

`ext_agent_host_id` 是客户端保存的实例 UUID；`oaiapp_...` 是授权签发的应用 client ID；subject 标识用户；token 连接授权与推理。这些是同一授权链上的身份信息，不等于同一网络指纹。

浏览器授权使用用户浏览器自己的 UA、TLS 和网络出口。token/JWKS/models 与推理使用服务端配置的代理、标准 Go 连接池和 `Sub2API-SIWC/1.0` UA。不同服务器/域名的 TLS 会话仍然不同。Responses 不伪造 host-ID 请求头，不复制浏览器 CF Cookie，也不保证所有流量拥有相同 IP/TLS 指纹。

## 持久化与回归

无需数据库 schema 迁移。凭据沿用现有账号 JSON 存储。自动刷新遵守 `earliest_refresh_at`，保留上游省略的 RT/scope，校验刷新后的身份。JWKS 在消耗授权码/RT 前获取，避免验证服务临时故障发生在旋转之后。

模型目录更新只原子修改 `siwc_models`；刷新/重新授权使用身份、旧 AT/RT 与代理条件更新，只合并授权字段并原子写调度失效事件。普通编辑在数据库行锁下保留最新凭据，防止旧表单覆盖新 token。旧刷新请求的失败不覆盖新授权状态。

本地验证包含：OAuth 签名与错误授权、回调过期/重复字段、重授权身份绑定、并发回填与保存重试、目录清空、凭据 CAS、后台隔离、Responses/Chat/Messages 的完整历史和 usage、真正提前输出的 SSE、重复 model 拒绝、计费档位、前端授权表单、中英文资源、普通 OAuth 回归。构建、类型检查、定向 race 测试、go vet 与仅本次差异的 golangci-lint 用于交付验收。

上线前仍需使用取得 SIWC 权限的测试账号做端到端授权、工具续接、实际 usage/账务和质量对照。此次本地测试不能证明线上可用、首字延迟改善或“百分百不降智”。

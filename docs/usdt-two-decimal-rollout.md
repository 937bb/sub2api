# B 端首页、加载状态与 USDT 两位小数发布记录

网站基线 `937sub2b` 的 `3332678938ba6e9d1ef4d971ce1e377adcb270a3` 已核对线上 B 端二进制版本。配套网关源码位于 `D:/myProject/epusdt-quote-gateway`，取自服务器候选源码 `/root/candidates/epusdt-quote-20260926-2ab197b/source`。原始源码压缩包 SHA-256 为 `4736531bec7c9e610233c317eb7d89fae6573312422ca42435a8241542d3b3d2`。网关提交 `a2aebce2a7fd8f6095c3a10b32916b361a95fd14`，完整补丁保存于 `deploy/patches/epusdt-two-decimal.patch`。不能直接用普通上游 master 替换定制网关。

网站在报价和下单签名中传入 amount_precision=2。新报价用 decimal.Round(2)，同地址占号按 0.01 递增。网站验证网关精度和真实应付额，输出固定两位。不传参数的旧客户端继续使用原精度，不改全局精度。旧订单查询和回调继续接受原金额；不能改写旧订单金额或批量重放已人工补款订单。最新兼容修正仅对明确请求两位的实时汇率订单比较 BEP20 链上原始整数单位，不能将少付金额四舍五入为足额。

前端 377 个测试文件、2915 项测试通过，类型检查和生产构建通过，桌面首页浅色/深色布局已检查。本机 Go 测试程序受到 Windows Application Control 限制，已在服务器独立目录、临时测试数据库中限额运行：网关 data、service、mq、bootstrap、middleware 测试通过；网站支付 provider 和 payment/notification/redeem 相关 unit 回归通过。

无感发布顺序：

1. 在线备份两个网关 SQLite 数据库，检查完整性，保存容器及代理配置。备份只在服务器保存，不提交 Git。
2. 新网关以备用回环端口启动，设置 EPUSDT_WORKERS_GATE_FILE。激活文件在旧网关退出前必须不存在；HTTP 可用，但链监听、回调与清理任务暂不启动。
3. 验证健康及新旧签名报价后，检查 Nginx 配置并平滑 reload。旧 HTTP 请求排空后，禁止旧容器自动重启，用 SIGINT 优雅退出。确认旧进程退出后才创建持久激活文件，启动新后台任务。此机制不是自动选主，不能让多个后台进程共用激活文件同时运行。
4. 网站使用新端口并行启动，保留 HTTP 入口代理与独立 WebSocket 路径；检查健康和页面资源后切换。保留旧连接自然排空，新镜像合并旧哈希静态资源，保护已打开页面的懒加载。
5. 回滚仅切代理和服务版本，不能恢复旧数据库覆盖新订单。网关回滚必须先停新后台进程、禁止其自动重启并移走激活文件，再启动旧后台进程。

## 2026-10-29 发布结果

网站镜像 `sub2api-b:ui-payment-a6685ef87`，容器 `sub2api-b-ui-payment`，监听 26242；入口代理 `sub2api-b-ui-ingress` 监听 26241。回环 6064 的 Nginx 默认 HTTP 上游已平滑切至 26241，WebSocket 上游仍为 26233。已打开页面所需旧哈希资源已合并进镜像。

网关镜像 `epusdt:two-decimal-20261029`，容器 `epusdt-two-decimal`，监听回环 8001。原网关已退出且禁止自动重启。站点配置使用回环 8000，因此发布时使用精确限定 127.0.0.1:8000 的临时 OUTPUT 转发维持新连接，旧连接排空后将 8000 改为 Nginx 代理；临时转发规则已移除。持久配置是 `/www/server/panel/vhost/nginx/epusdt-loopback-8000.conf`，代理至 8001。工作进程激活文件为 `/www/wwwroot/epusdt.cmsnav.com/app/runtime/workers-20261029.enabled`。

切换期间 1194 次网站/网关健康探测全部返回 200，观察窗口内最长 12ms。线上首页已用浏览器核验；BSC 扫描游标持续推进。签名报价验证旧请求返回 6 位、新请求返回 2 位。未发起真实链上付款，因此没有宣称完成真实付款端到端验收。

服务器发布目录 `/root/candidates/b-ui-payment-20261029` 包含构建日志、在线数据库备份、配置备份和探测记录；`compose.active.yaml` 与 `release.json` 记录当前三个新容器，环境文件保存在权限受限的 backup 目录。历史 `/www/wwwroot/sub2api-b/docker-compose.yml` 仍属旧版本，不能直接用它重建当前发布。旧 HTTP 实例保留以排空连接，独立 WebSocket 实例继续使用原镜像。GitHub 由本机直连推送。该用户已手工补余额，本次没有更改其余额或订单状态。

## 2026-10-30 共享网关兼容修正

网关提交 `3ef49cccc00375b4678e3d1a86f53a41e4846495`；`deploy/patches/epusdt-two-decimal.patch` 已更新为相对原始生产源码 `87d8723` 的完整补丁。前一版 `a2aebce` 将严格金额匹配扩大到所有项目，本次撤回该全局变化。

- B 端仍发送人民币金额和签名参数 `rate_policy=live, amount_precision=2`，网关换算后生成两位 USDT 应付额。仅把人民币输入写成两位，不能保证换算后的 USDT 也是两位。
- 未显式请求新精度的客户端恢复原有金额舍入、按锁定精度匹配收款以及切链汇率逻辑；全局精度仍为 6。
- 只有已持久化为 `rate_policy=live, amount_precision=2` 的订单采用新规则，包括切链子订单。旧固定汇率两位订单也保留原来的匹配行为，无须改写历史订单或数据库结构。
- 回归覆盖旧 2/4/6 位锁、六位实时汇率订单、B 端两位混合占号、少付、链上原始单位、重复处理、过期订单回查及切链兼容。网关 data/service/mq/bootstrap/middleware 测试通过。

未发起真实资金转账，测试与签名报价不能代替其他项目真实充值验收。已经人工补款的历史订单保持原样。

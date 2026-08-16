# 首字速度运维优化方案（WS + 多实例 + BBR/TCP）

放弃 res→chat 协议转换，改从运维层压低首字延迟。三层，按性价比从高到低。

首字延迟的构成：
```
客户端 → [Caddy TLS] → [网关] → [到 chatgpt.com 建连/TLS/首包] → 上游首 token
                                   ↑ 大头在这：每次新建 WS/TLS 握手 200~500ms
```
核心思路：**把"每次现建连接"变成"复用已预热的长连接"**，再用内核 TCP 调优压低握手和传输 RTT。

---

## 一、内核 BBR + TCP 调优（性价比最高，宿主机执行）

BBR 拥塞控制在跨境高 RTT + 丢包链路上，比默认 cubic 显著降低延迟、提高吞吐。这是"解锁 TCP 性能"的关键——你说的"堵连接"通常是 backlog 太小 + 没开 BBR。

**容器部署也在宿主机执行**（容器共享宿主内核）。写入 `/etc/sysctl.d/99-sub2api-perf.conf`：

```conf
# ---- BBR 拥塞控制 + fq 队列（BBR 必须配 fq/fq_codel）----
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr

# ---- 连接 backlog：解决"堵连接"（并发建连被丢）----
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.netdev_max_backlog = 32768

# ---- 端口/连接复用：高并发建连必备 ----
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15

# ---- 首字相关：关掉慢启动重置 + 快速握手 ----
net.ipv4.tcp_slow_start_after_idle = 0   # 空闲后不重置拥塞窗口（长连接复用关键）
net.ipv4.tcp_fastopen = 3                # TFO：省一次 RTT
net.ipv4.tcp_notsent_lowat = 16384       # 降低发送缓冲堆积，流式更早吐字

# ---- 缓冲区：跨境大 BDP 链路 ----
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216

# ---- keepalive：尽早发现死连接，避免打到僵连接超时 ----
net.ipv4.tcp_keepalive_time = 60
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 3

# ---- 文件句柄（大量长连接）----
fs.file-max = 2097152
```

应用与验证：
```bash
sudo sysctl --system
sysctl net.ipv4.tcp_congestion_control   # 应输出 bbr
sysctl net.core.default_qdisc            # 应输出 fq
lsmod | grep bbr                         # tcp_bbr 已加载
```

内核 <4.9 没有 BBR；`modprobe tcp_bbr` 若失败需升级内核。容器还需放开句柄上限：compose 里给 backend 加
```yaml
    ulimits:
      nofile: {soft: 1048576, hard: 1048576}
```

---

## 二、WS 预热 + 连接池（网关层，直接砍首字握手）

`responses_websockets_v2` 默认已开，但**预热默认关**——首个请求仍要现建 WS，握手延迟全落在首字上。打开预热 + 调大空闲池，让请求进来时连接已就绪。

`config.yaml` 的 `gateway.openai_ws`：

```yaml
gateway:
  openai_ws:
    enabled: true
    responses_websockets_v2: true

    # 预热：提前建好并保活连接，请求进来直接复用（首字关键）
    prewarm_generate_enabled: true      # 默认 false → 改 true
    prewarm_cooldown_ms: 300

    # 每账号空闲池：调大下限，避免冷启动现建
    min_idle_per_account: 8             # 默认 4 → 8（按并发调）
    max_idle_per_account: 24            # 默认 12 → 24
    max_conns_per_account: 128
    dynamic_max_conns_by_account_concurrency_enabled: true

    # 拨号超时收紧：坏连接快速失败换下一条，不拖首字
    dial_timeout_seconds: 8             # 默认 10 → 8

    # 会话保活：拉长 turn 间空闲存活，复用同一条连接
    ingress_inter_turn_idle_timeout_seconds: 300
```

调参原则：
- `min_idle_per_account` ≈ 单账号常见并发。号池大时预热成本上升，按"活跃账号数 × min_idle"估算连接总量，配合内核句柄上限。
- 号池很大（几千账号）别全量预热——只对**高优先级/超额账号**预热。可结合调度让热账号常驻。

---

## 三、多实例（横向扩，摊 CPU + 连接数）

单实例 8080 是瓶颈：TLS 解密、SSE 转发、连接池都挤一个进程。多实例 + Caddy 负载均衡分摊。

`docker-compose.yml`：
```yaml
  backend:
    image: weishaw/sub2api:latest
    deploy:
      replicas: 3                       # 起 3 个实例
    # 或不用 swarm：复制成 backend-1/2/3 三个 service
    ulimits:
      nofile: {soft: 1048576, hard: 1048576}
```

Caddy 反代改指向多实例（已有 `lb_policy round_robin`）：
```
reverse_proxy backend-1:8080 backend-2:8080 backend-3:8080 {
    lb_policy least_conn        # 流式长连接用 least_conn 比 round_robin 更均衡
    ...
}
```

**多实例注意**：
- WS 连接池是**进程内**的——3 个实例各自维护预热池，同一账号的连接会分散在 3 个进程。sticky 调度（按 group/account）能让同账号请求落同实例，提高复用率。Caddy 加 `lb_policy` 的 sticky：
  ```
  lb_policy cookie sub2api_lb
  ```
  或按上游账号哈希（需网关侧支持）。
- Redis/PG 是共享的，多实例无状态一致。

---

## 落地顺序（建议）

1. **先做内核 BBR/TCP**（一次性、全局收益、零代码）→ 立刻验证 `curl -w "%{time_starttransfer}"` 首字变化
2. **再开 WS 预热**（config.yaml 改 3 个值 + 重启）→ 对比首字
3. **最后上多实例**（并发大了再横向扩）

## 验证首字的命令

```bash
# time_starttransfer = 首字节到达时间（首字核心指标）
curl -o /dev/null -s -w "建连=%{time_connect}s TLS=%{time_appconnect}s 首字=%{time_starttransfer}s 总=%{time_total}s\n" \
  -X POST https://api.sub2api.com/v1/responses \
  -H "Authorization: Bearer sk-xxx" -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"hi"}],"stream":true}'
```
优化前后各跑 10 次取中位数对比 `首字`。

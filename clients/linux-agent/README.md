# Linux Agent

Linux Agent 是当前第一个常驻节点守护进程子项目，负责：

- 节点注册
- 周期心跳
- Bootstrap 同步
- 本地 overlay runtime 编译
- 本地状态和配置持久化
- secure-udp 数据面密钥托管与加密传输

## 快速开始

```bash
go run ./cmd/linux-agent init-config
go run ./cmd/linux-agent enroll --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent run --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent runtime-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent plan-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent apply-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent session-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent session-report --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent dataplane-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent transport-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent direct-attempt-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent direct-attempt-report --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent recovery-status --config ~/.config/nodeweave/linux-agent.json
go run ./cmd/linux-agent stun-status --config ~/.config/nodeweave/linux-agent.json
```

## 文件

- `configs/linux-agent.example.json`：配置样例
- `deploy/systemd/nodeweave-linux-agent.service`：systemd 服务样例

## 身份和密钥

- 如果配置了 `private_key_path`，agent 首次启动会自动生成静态 X25519 私钥文件
- 注册到控制面的 `public_key` 会从这把私钥派生，不需要手填
- `secure-udp` 数据面也复用这把私钥
- 如果更换了私钥文件，agent 会在后续 heartbeat 中把新的 `public_key` 同步到控制面
- 其他节点会在下次 bootstrap 同步时拿到新的 peer 公钥

## 当前数据面实现边界

当前 runtime 先使用 `dry-run` backend：

- 根据 bootstrap 生成接口、peer、路由、DNS 的本地 runtime snapshot
- 将 snapshot 写入配置中的 `runtime_path`
- 暂不直接创建真实 TUN 或系统路由
- 同时会编译 peer session 候选路径图，写入 `session_path`
- 同时会编译 dataplane 路由和候选出口映射，写入 `dataplane_path`

如果将 `apply_mode` 改成 `linux-plan`：

- agent 会额外生成 Linux 网络编排计划
- 计划写入配置中的 `plan_path`
- 计划内容基于 `ip` 和 `resolvectl` 命令
- 当前会覆盖接口、peer 主机路由、静态路由、DNS 和出口节点默认路由

如果将 `apply_mode` 改成 `linux-exec`：

- agent 会直接执行 Linux 网络编排计划中的命令
- 每次 apply 的执行结果会写入 `apply_report_path`
- 默认要求 root，行为由 `exec_require_root` 控制
- 单条命令超时由 `exec_command_timeout` 控制
- 执行前会探测接口、地址、路由和 DNS 当前状态，已满足项会标记为 `skipped`

如果将 `session_probe_mode` 改成 `udp`：

- agent 会对 peer 的直连 endpoint 和 relay 地址执行最小 UDP 探测
- 最新探测结果写入 `session_report_path`
- 如果探测到了可达的 relay 或备用 endpoint，后续 dataplane 编译会优先切到这个 candidate
- `run` 模式下如果配置了 `session_listen_address`，会启动本地 UDP responder

如果配置了 `stun_servers`：

- agent 会在每次 heartbeat 前向 STUN server 发起最小 Binding discovery
- 发现到的 reflexive endpoint 会和静态 `advertise_endpoints` 一起上报控制面
- 如果 dataplane 已经启动，agent 也会把当前实际监听地址作为 `listener` 来源一起上报
- heartbeat 同时会上报 `endpoint_records`，区分 `static` 和 `stun` 来源，并带上最近观测时间
- heartbeat 还会上报最小 NAT 摘要，包括 `mapping_behavior`、sample 数量、当前选中的 reflexive address 和可达性
- heartbeat 也会上报每个 peer 的当前 transport 摘要，包括 `active_kind`、`active_address` 和最近一次 direct attempt 结果
- 如果 heartbeat 响应里的 `bootstrap_version` 比本地新，agent 会立即拉取新的 bootstrap、重编 runtime 并重载 dataplane
- 如果 heartbeat 响应里带有 `direct_attempts`，agent 会在本地短期调度队列里按 `execute_at/window/burst_interval` 执行 coordinated direct burst；控制面会优先把 relay 活跃链路下发为 `relay_active`
- 这些未执行完的 `direct_attempts` 会写入 `direct_attempt_path`；如果 dataplane 暂时没起来，或者 agent / secure-udp runtime 重启了，只要 attempt 还没过期，就会在 transport 恢复后继续调度
- `direct-attempt-status` 可直接查看当前还在本地排队、等待执行或等待 transport 恢复的 coordinated direct attempts
- `direct-attempt-report` 会保留最近一段 attempt 生命周期，包括 `queued`、`waiting_transport`、`scheduled`、`executing`、`completed`、`expired`，以及 controlplane `issued_at`、等待原因、最后错误和命中的地址
- 如果 controlplane 最新 recovery decision 已经把某个旧 attempt 判定为 block、direct 已恢复，或被更新的 attempt 替代，agent 会主动取消这个 stale attempt，并在 report 里保留取消原因
- `recovery-status` 现在除了 block 原因、`next_probe_at` 和 probe 配额，还会带最近一次 controlplane 放行的 direct attempt ID / reason / `issued_at` / `execute_at`
- 即使 controlplane 这轮没有下发新的 `direct_attempt`，`recovery-status` 也会显示 `decision_status / decision_reason / decision_at / decision_next_at`，用于解释为什么当前没有被调度恢复，以及最早何时会重新评估
- 如果最近一次 direct attempt 结果已经上报为 `timeout` 或 `relay_kept`，控制面会进入短暂冷却窗口，避免 agent 被连续打洞指令刷屏；这两个结果现在可以配置不同的 cooldown
- 冷却结束后如果 relay 仍持续活跃，控制面下发的后续恢复指令会标记为 `manual_recover`，并可使用比普通 `fresh_endpoints` 更激进的独立时间窗；`timeout` 和 `relay_kept` 也可以配置不同的升级阈值
- heartbeat 上报的 peer transport 摘要现在还会带最近一次 direct success 时间和连续失败次数，控制面可以据此在失败预算耗尽后临时抑制恢复
- heartbeat 响应里的 `peer_recovery_states` 会落到本地 `recovery_state_path`，`recovery-status` 可直接查看当前哪些 peer 被 controlplane block、block 原因、截止时间、`next_probe_at` 和剩余 probe 配额
- 如果 suppression 仍未结束，但 controlplane 配置了 `*_SUPPRESSED_PROBE_INTERVAL`，后续 heartbeat 里仍可能收到一小批 `manual_recover` 指令做稀疏恢复尝试
- 如果 controlplane 同时配置了 `*_SUPPRESSED_PROBE_LIMIT`，当剩余 probe 配额耗尽后，agent 将只看到 block 状态更新，不会再收到新的 suppressed probe 恢复指令
- 如果 controlplane 还配置了 `*_SUPPRESSED_PROBE_REFILL_INTERVAL`，`recovery-status` 里会额外显示 `probe_refill_at`，表示 quiet period 之后配额何时自动恢复
- 后台 `direct_warmup_interval` 预热也会消费这些 recovery states；如果某个 peer 仍被 block，本地 warmup 会一直暂停到 `next_probe_at` 或 `blocked_until`
- 新的 heartbeat 把 recovery states 更新到本地后，warmup 循环会立即被唤醒重算，而不是继续睡到旧的 wait 截止时间
- 最新 STUN 结果会写入 `stun_report_path`
- `stun-status` 可查看各个 server 的可达性、RTT 和当前选中的 reflexive address
- 当 `secure-udp` 数据面已经运行时，STUN 会复用同一个 UDP 监听 socket，对外发现出的 reflexive port 会和真实数据面端口一致；只有数据面还没启动时才回退到独立探测 socket
- `direct_warmup_interval` 大于 `0s` 时，agent 会在后台持续对 peer 的 direct candidate 发起 secure-udp 握手预热；实际重试时间会跟随 transport report 里的 `next_direct_retry_at`，而不是死等一个固定全局周期
- direct 建链时，secure-udp 会在一个握手窗口内对多个 direct candidate 连续发起 `hello` burst，提高 NAT 场景下的直连成功率
- 每次 bootstrap 驱动的 dataplane reload 成功后，agent 会立即补发一次 heartbeat，让新的监听地址更快传播到控制面

如果将 `dataplane_mode` 改成 `udp`：

- `run` 模式下会启动最小 UDP dataplane listener
- 当前实现会接收 frame 并记录日志，作为后续 TUN packet pump 的接入点

如果将 `dataplane_mode` 改成 `secure-udp`：

- `run` 模式下会启动带静态节点身份的加密 UDP dataplane listener
- peer 首次收发前会执行 `hello` / `hello_ack` 握手
- 负载使用 X25519 派生共享密钥，再用 AES-GCM 封装
- 传输层带 nonce 重放保护
- 如果本地没有 `private_key_path` 指向的私钥文件，会自动生成
- 如果 session 候选里包含 relay 地址，agent 会周期性向 relay 发送 `announce` 刷新映射
- 如果当前 direct 候选握手失败或发送失败，传输层会在同一轮发送里自动回退到 relay 候选
- 如果当前活跃路径已经是 relay，传输层会周期性重试 direct，并在恢复后自动切回 direct
- 如果还没有真实业务流量，agent 也会按 `direct_warmup_interval` 对 direct candidate 主动做握手预热
- 运行中的活动路径、候选列表、最近握手和下一次 direct 重试时间会写入 `transport_report_path`
- `transport-status` 还会暴露每个 peer 的最后一次 direct attempt 的 ID/原因/结果、收发包计数、最后一次发送错误、handshake timeout 次数，以及 relay fallback / direct recovery 次数

如果将 `tunnel_mode` 改成 `linux`：

- agent 会尝试打开 Linux TUN 设备并启动 packet pump
- outbound packet 会按 dataplane 路由表转成 UDP frame 发往 peer 候选地址
- inbound frame 会回写到 TUN 设备

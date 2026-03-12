# 企业级异地组网系统

本仓库当前采用 monorepo 结构，已经包含可运行的控制面、共享契约模块和 Linux CLI 客户端基线。

## 文档索引

- [产品需求文档](docs/prd.md)
- [系统架构文档](docs/architecture.md)
- [需求追踪与开源借鉴矩阵](docs/reference-map.md)
- [开发任务拆分](docs/task-breakdown.md)
- [Codex 多 Agent 开发](docs/codex-multi-agent-team.md)

多 Agent 本地编排运行时，当前阶段可查看：
- `/home/hao/AAA/NodeWeave/.codex-team-runs/latest/current_phase.txt`

## 仓库结构

- [services](services/README.md)：服务端子项目
- [clients](clients/README.md)：桌面端和 CLI 子项目
- [mobile](mobile/README.md)：Android / iOS 子项目
- [gateways](gateways/README.md)：OpenWrt 和边缘桥接子项目
- [packages](packages/README.md)：共享协议与 SDK
- [docs](docs/prd.md)：需求、架构和任务拆分文档

## 建议阅读顺序

1. 先阅读 `docs/prd.md`，确认业务目标、范围和版本边界。
2. 再阅读 `docs/architecture.md`，统一控制面、数据面、客户端版本和兼容策略。
3. 阅读 `docs/reference-map.md`，确认每个能力模块的开源借鉴对象和当前实现边界。
4. 最后阅读 `docs/task-breakdown.md`，按 Epic 和里程碑推进研发落地。

## 当前决策摘要

- 产品形态采用“控制面 + Overlay 数据面 + P2P/Relay 混合链路”。
- 跨平台策略采用“共享网络核心 + 平台壳层”模式。
- 与 WireGuard、Tailscale、ZeroTier、EasyTier 的“兼容”定义为网络互通与桥接，不承诺控制面或协议级无缝互认。
- 第一阶段以 Windows、Linux、控制服务器、Relay 和最小可用 NAT 穿透为目标。

## 当前代码状态

当前已落下三个可构建子项目：

- `services/controlplane/cmd/controlplane`：控制面启动入口
- `services/relay/cmd/relay`：UDP relay 启动入口
- `services/controlplane/internal/httpapi`：REST API
- `services/controlplane/internal/store`：SQLite 持久化和内存实现
- `clients/linux-agent/cmd/linux-agent`：Linux 常驻节点代理
- `clients/linux-cli/cmd/linux-cli`：Linux CLI 注册、状态查询、心跳
- `packages/runtime/go`：共享 overlay runtime 和 driver 抽象
- `deployments/local/docker-compose.yml`：本地容器化部署
- `scripts/e2e_smoke.sh`：端到端冒烟验证
- `.github/workflows/ci.yml`：CI 构建测试与镜像构建
- `packages/contracts/go`：共享 API 类型和 Go HTTP client
- `services/controlplane/configs/controlplane.env.example`：本地开发环境变量示例

## 快速启动

```bash
export $(grep -v '^#' services/controlplane/configs/controlplane.env.example | xargs)
go run ./services/controlplane/cmd/controlplane
```

另开一个终端注册 Linux CLI：

```bash
go run ./clients/linux-cli/cmd/linux-cli enroll --server http://127.0.0.1:8080
go run ./clients/linux-cli/cmd/linux-cli status
go run ./clients/linux-cli/cmd/linux-cli login --server http://127.0.0.1:8080
go run ./clients/linux-agent/cmd/linux-agent init-config
go run ./clients/linux-agent/cmd/linux-agent runtime-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent apply-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent session-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent session-report --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent dataplane-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent transport-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent direct-attempt-status --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent direct-attempt-report --config ~/.config/nodeweave/linux-agent.json
go run ./clients/linux-agent/cmd/linux-agent stun-status --config ~/.config/nodeweave/linux-agent.json
```

默认监听地址：

- `:8080`

当前可用 API：

- `GET /healthz`
- `POST /api/v1/auth/login`
- `POST /api/v1/devices/register`
- `GET /api/v1/nodes`
- `GET /api/v1/nodes/{id}/bootstrap`
- `POST /api/v1/nodes/{id}/heartbeat`
- `GET /api/v1/routes`
- `POST /api/v1/routes`
- `GET /api/v1/dns/zones`

当前 Linux 数据面能力：

- `linux-plan` 可生成接口、peer 主机路由、静态路由、DNS 和出口节点默认路由计划
- `linux-exec` 会在执行前探测接口、地址、路由和 DNS 当前状态，已满足项会跳过
- `session` 运行时会编译 peer 直连/Relay 候选路径，并支持最小 UDP 连通性探测
- `stun` 运行时会向配置的 STUN server 发现 reflexive endpoint，并在 heartbeat 时上报控制面
- 当 `secure-udp` 数据面已经启动时，STUN 会优先复用同一个 UDP 监听 socket，避免“探测出来的外网端口”和真实数据面端口不一致
- heartbeat 现在还会附带最小 NAT 摘要，包括 `mapping_behavior`、`selected_reflexive_address`、sample 数量和可达性，供控制面生成协同打洞指令
- heartbeat 也会上报每个 peer 的当前 transport 摘要，至少包含 `active_kind`、`active_address` 和最近一次 direct attempt 结果
- `linux-agent` 还会把当前 dataplane 实际监听地址作为 `listener` 来源上报控制面，并在 dataplane reload 后立即补发 heartbeat
- `linux-agent` 收到更高的 `bootstrap_version` 心跳响应时，会立即刷新 bootstrap/runtime/dataplane，而不是等待下一轮 bootstrap 轮询
- 控制面 bootstrap 里的 peer endpoint 现在会保留来源和观测时间，session 编译优先使用最新的 STUN / static candidate，而不是一串无序裸地址
- 控制面会基于双方最新 heartbeat、endpoint freshness 和 peer transport 摘要下发一次性 `direct_attempts`，并优先把当前 relay 活跃的链路标成 `relay_active`
- 如果任一侧刚上报了 `timeout` / `relay_kept` 这类 direct attempt 失败结果，控制面会进入短暂冷却窗口，避免连续抖动重试
- 这些窗口现在都可以通过控制面环境变量调节，包括在线判定、endpoint freshness、按失败类型区分的 cooldown、按失败类型区分的 `manual_recover` 升级阈值、按失败类型区分的失败预算与 suppression window、attempt window，以及 `relay_active` / `primary_upgrade` / `manual_recover` 各自独立的 lead/window/burst profile
- suppression 生效期间，controlplane 还可以按稀疏 probe interval 重新放行一次 `manual_recover`，避免在整个 block window 内完全沉默
- 稀疏 probe 现在还支持按失败结果分层的 probe limit，避免在长 suppression window 里无限次放行恢复尝试
- probe limit 也支持 quiet-period 自动回补，controlplane 会在 `peer_recovery_states` 和 bootstrap peer 摘要里暴露 `probe_refill_at`
- controlplane 还会把每个 peer 当前的恢复 block 状态通过 `peer_recovery_states` 回给 agent，并在 bootstrap peer 摘要里暴露 block 原因、截止时间、下一次 probe 时间和剩余 probe 配额
- `secure-udp` 在 direct 建链时会在一个握手窗口内跨多个 direct candidate 重复发送 `hello` burst，帮助 relay 活跃期间更主动地恢复直连
- controlplane 协调下发的 `direct_attempt` 现在会在 `execute_at` 前的一个很短 prewarm lead 内就开始 hello burst，而不是严格等到整点才发第一包，从而更接近真实 simultaneous-open
- `linux-agent` 会消费控制面返回的 `direct_attempts`，到点调用显式 `ExecuteDirectAttempt(...)`，失败时保持现有 relay 活跃路径
- `direct_attempts[*].candidates` 现在是结构化对象而不是裸地址，至少包含 `address/source/observed_at/priority/phase`
- controlplane 会把 fresh `listener/stun` 划入 `primary` phase，把 `static/heartbeat` 划入 `secondary` phase；`secure-udp` 会先跑 `primary`，只有在 `execute_at` 已到且未建链时才进入 `secondary`
- 如果上一轮恢复只打到了 `secondary`，而控制面已经看到了比上次失败更新的 fresh `primary` candidate，则下一轮会自动切到更激进的 `primary_upgrade` profile，而不是继续沿用普通 `fresh_endpoints` / `relay_active` 时间窗
- `direct_attempts[*]` 现在还会带 `profile`，agent 本地 report、heartbeat 上报的 `peer_transport_states` 和 bootstrap peer 摘要也会保留最近一次命中的 profile，便于区分“普通恢复”还是 `primary_upgrade`
- `primary_upgrade` 失败后的 cooldown、manual_recover 升级阈值和 suppression/probe 参数现在也可以独立配置；未单独设置时仍会回退到普通 `timeout` / `relay_kept` 恢复治理
- `linux-agent` 会把未执行完的 `direct_attempts` 持久化到本地；dataplane 未就绪或 agent 重启后，只要 attempt 还没过期，就会在 secure-udp transport 恢复时继续调度
- `linux-agent direct-attempt-status` 可直接查看这些仍在本地排队的 coordinated direct attempts
- `linux-agent direct-attempt-report` 会保留最近一批 attempt 的生命周期、controlplane `issued_at`、当前 phase、命中的 source 和等待原因，便于判断它是卡在 transport 不可用、正常排程、执行中，还是已经 timeout / relay_kept / success / expired
- heartbeat 上报给 controlplane 的 `peer_transport_states` 现在也会带最近一次 direct attempt 命中的 `source`、执行到的 `phase` 和本次使用的 candidate 数量；bootstrap peer 摘要会原样暴露这些观测值
- 如果 controlplane 最新 `peer_recovery_states` 已经表明某个旧 attempt 被 block、被新 attempt 替代，或者链路已经恢复 direct，agent 会主动取消并移除这类 stale attempt，并在 report 里写出取消原因
- controlplane 返回的 `peer_recovery_states` 和 bootstrap peer 摘要现在也会暴露最近一次放行的 direct attempt ID / reason / `issued_at` / `execute_at`
- 即使这次没有下发新的 direct attempt，controlplane 也会在 `peer_recovery_states[*].decision_status / decision_reason / decision_at / decision_next_at` 以及 bootstrap peer 摘要里明确说明当前为什么被 block、为什么保持 direct、为什么因 peer offline / 缺少 fresh direct candidate 而没调度，以及最早何时会重新评估
- `linux-agent` 后台 `direct_warmup_interval` 预热现在也会遵守 controlplane 返回的 `peer_recovery_states`，在 `next_probe_at` / `probe_refill_at` 之前暂停本地 warmup
- heartbeat 把 recovery state 更新到本地后，后台 warmup 会立即重算，不必等上一个 sleep 周期结束
- `linux-agent` 会在后台对 direct candidate 主动发起 secure-udp 握手预热，并根据 transport report 暴露的 `next_direct_retry_at` 精准安排下一轮恢复尝试，减少等待真实流量才建链的延迟
- `dataplane` 运行时会编译目标网段到 peer/endpoint 的映射，并消费最新 session probe 结果在 direct/relay candidate 之间切换
- `secure-udp` 数据面已支持静态 X25519 节点身份、`hello` / `hello_ack` 握手、AES-GCM 封装、nonce 重放保护，以及 direct 失败后的 relay 回退与 direct 恢复切回
- `secure-udp` 运行时会持续写出 transport report，暴露当前 active path、候选列表、最近握手时间、direct 重试窗口、最后一次 direct attempt 的 ID/原因/结果，以及每个 peer 的收发计数和 fallback/recovery 统计
- `relay` 服务已支持基于 `source_node_id` 的 UDP 地址映射和 `secure-udp` 报文的透明转发
- `tunnel` 运行时已提供 Linux TUN 设备和 packet pump 骨架，可把 TUN packet 接到 dataplane transport

## 常用命令

```bash
make fmt
make test
make build
make e2e
make run-controlplane
```

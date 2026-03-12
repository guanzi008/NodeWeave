# Control Plane

当前控制面子项目提供一个可运行的 MVP API。

## 目录

- `cmd/controlplane`：服务启动入口
- `internal/config`：环境变量配置
- `internal/httpapi`：HTTP API
- `internal/store`：存储接口、SQLite 和内存实现
- `configs/controlplane.env.example`：本地开发配置样例

## 运行

```bash
cd services/controlplane
export $(grep -v '^#' configs/controlplane.env.example | xargs)
go run ./cmd/controlplane
```

默认使用 SQLite 持久化：

- `CONTROLPLANE_STORAGE_DRIVER=sqlite`
- `CONTROLPLANE_SQLITE_PATH=data/controlplane.db`

可选出口节点下发：

- `CONTROLPLANE_EXIT_NODE_ID=node_xxx`
- `CONTROLPLANE_EXIT_NODE_MODE=enforced`
- `CONTROLPLANE_EXIT_NODE_ALLOW_LAN=true`
- `CONTROLPLANE_EXIT_NODE_ALLOW_INTERNET=true`
- `CONTROLPLANE_EXIT_NODE_DNS_MODE=follow_exit`

可选协同打洞策略参数：

- `CONTROLPLANE_NODE_ONLINE_WINDOW=30s`
- `CONTROLPLANE_ENDPOINT_FRESHNESS_WINDOW=45s`
- `CONTROLPLANE_TRANSPORT_FRESHNESS_WINDOW=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_COOLDOWN=10s`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_COOLDOWN=10s`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_COOLDOWN=10s`
- `CONTROLPLANE_DIRECT_ATTEMPT_LEAD=150ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_WINDOW=600ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_BURST_INTERVAL=80ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_RETENTION=2s`
- `CONTROLPLANE_DIRECT_ATTEMPT_MANUAL_RECOVER_AFTER=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_MANUAL_RECOVER_AFTER=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_MANUAL_RECOVER_AFTER=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_FAILURE_SUPPRESS_AFTER=4`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_SUPPRESS_AFTER=4`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_SUPPRESS_AFTER=6`
- `CONTROLPLANE_DIRECT_ATTEMPT_FAILURE_SUPPRESS_WINDOW=2m`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_SUPPRESS_WINDOW=2m`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_SUPPRESS_WINDOW=2m`
- `CONTROLPLANE_DIRECT_ATTEMPT_SUPPRESSED_PROBE_INTERVAL=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_SUPPRESSED_PROBE_INTERVAL=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_SUPPRESSED_PROBE_INTERVAL=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_SUPPRESSED_PROBE_LIMIT=2`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_SUPPRESSED_PROBE_LIMIT=2`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_SUPPRESSED_PROBE_LIMIT=2`
- `CONTROLPLANE_DIRECT_ATTEMPT_SUPPRESSED_PROBE_REFILL_INTERVAL=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_TIMEOUT_SUPPRESSED_PROBE_REFILL_INTERVAL=30s`
- `CONTROLPLANE_DIRECT_ATTEMPT_RELAY_KEPT_SUPPRESSED_PROBE_REFILL_INTERVAL=30s`
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_LEAD=200ms`
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_WINDOW=900ms`
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_BURST_INTERVAL=60ms`
- `CONTROLPLANE_SECONDARY_ONLY_ATTEMPT_LEAD=300ms`
- `CONTROLPLANE_SECONDARY_ONLY_ATTEMPT_WINDOW=1800ms`
- `CONTROLPLANE_SECONDARY_ONLY_ATTEMPT_BURST_INTERVAL=45ms`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_LEAD=200ms`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_WINDOW=900ms`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_BURST_INTERVAL=60ms`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_COOLDOWN=20s`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_MANUAL_RECOVER_AFTER=45s`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_SUPPRESS_AFTER=3`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_SUPPRESS_WINDOW=3m`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_SUPPRESSED_PROBE_INTERVAL=45s`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_SUPPRESSED_PROBE_LIMIT=1`
- `CONTROLPLANE_PRIMARY_UPGRADE_ATTEMPT_SUPPRESSED_PROBE_REFILL_INTERVAL=90s`
- `CONTROLPLANE_MANUAL_RECOVER_ATTEMPT_LEAD=250ms`
- `CONTROLPLANE_MANUAL_RECOVER_ATTEMPT_WINDOW=1500ms`
- `CONTROLPLANE_MANUAL_RECOVER_ATTEMPT_BURST_INTERVAL=50ms`

当前默认策略：

- relay 活跃链路优先触发 `relay_active`
- direct timeout / relay_kept 后进入冷却窗口，避免连续打洞
- `timeout` 和 `relay_kept` 可以分别配置不同 cooldown；未单独设置时回退到 `CONTROLPLANE_DIRECT_ATTEMPT_COOLDOWN`
- 冷却结束后，如果 relay 仍持续活跃，则后续尝试会标记成 `manual_recover`
- `timeout` 和 `relay_kept` 也可以分别配置不同的 `manual_recover` 升级阈值；未单独设置时回退到 `CONTROLPLANE_DIRECT_ATTEMPT_MANUAL_RECOVER_AFTER`
- direct attempt 连续失败达到预算后，控制面会在 suppression window 内暂停继续恢复；`timeout` 和 `relay_kept` 可以分别配置不同的失败预算和抑制窗口
- suppression 生效期间也可以配置稀疏恢复探测窗口；达到 `*_SUPPRESSED_PROBE_INTERVAL` 后，控制面会重新放行一次 `manual_recover`，而不是一直等到 block 彻底过期
- 稀疏恢复探测还可以再加一层 probe limit；达到 `*_SUPPRESSED_PROBE_LIMIT` 后，即使 suppression window 还没结束，也不会再继续放行 probe
- probe limit 现在还支持 quiet-period 自动回补；达到 `*_SUPPRESSED_PROBE_REFILL_INTERVAL` 后，已消耗的 probe 配额会逐步恢复
- `relay_active` 和 `manual_recover` 可以使用独立的 lead/window/burst profile，不必和 `fresh_endpoints` 共用一套时间窗
- 如果当前只有 `secondary` direct candidates，controlplane 会自动切到 `secondary_only` profile；这套 profile 只改打洞时间窗，不会改现有 cooldown / suppression 语义
- 如果 controlplane 观察到上一轮 direct attempt 只打到了 `secondary` phase，而且当前出现了比上次失败更晚的 fresh `primary` candidate，则会自动切到 `primary_upgrade` profile；未单独配置时，这套 profile 默认回退到 `relay_active`
- `primary_upgrade` 失败后的 cooldown、manual_recover 阈值和 suppression/probe 策略现在也可以独立配置；未单独设置时，仍会回退到当前 `timeout` / `relay_kept` 的治理参数
- `direct_attempts[*].candidates` 现在按结构化对象下发，包含 `address/source/observed_at/priority/phase`；controlplane 会把 fresh `listener/stun` 划成 `primary`，把 `static/heartbeat` 划成 `secondary`
- `direct_attempts[*]` 现在还会带 `profile`，用来说明这次下发到底走的是 `fresh_endpoints`、`relay_active`、`manual_recover`、`secondary_only` 还是 `primary_upgrade` 时间窗
- heartbeat 里的 `peer_transport_states` 现在也会带最近一次 direct attempt 命中的 `profile`、`source`、执行到的 `phase` 和 candidate 数量；这些字段会透传到 bootstrap peer 摘要，便于诊断当前恢复策略究竟命中了哪一类 direct candidate
- 当前 block 状态会同时反映到 `HeartbeatResponse.peer_recovery_states` 和 bootstrap peer 摘要里，包含 `next_probe_at`、`probe_budget`、`probe_failures`、`probe_remaining` 和 `probe_refill_at`
- 最近一次 controlplane 放行的 direct attempt trace 现在也会保留 `profile`，并通过 `peer_recovery_states` 和 bootstrap peer 摘要暴露
- 即使 controlplane 没有下发新的 `direct_attempts`，`peer_recovery_states` 和 bootstrap peer 摘要也会给出 `decision_status / decision_reason / decision_at / decision_next_at`，用于解释为什么这轮没有恢复直连，以及最早何时会重新评估

## 测试

```bash
cd services/controlplane
go test ./...
```

## Docker

```bash
docker build -f services/controlplane/Dockerfile -t nodeweave-controlplane .
docker run --rm -p 8080:8080 -v $(pwd)/data:/data nodeweave-controlplane
```

## Docker Compose

```bash
docker compose -f deployments/local/docker-compose.yml up --build
```

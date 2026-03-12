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
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_LEAD=200ms`
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_WINDOW=900ms`
- `CONTROLPLANE_RELAY_ACTIVE_ATTEMPT_BURST_INTERVAL=60ms`
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
- `relay_active` 和 `manual_recover` 可以使用独立的 lead/window/burst profile，不必和 `fresh_endpoints` 共用一套时间窗
- 当前 block 状态会同时反映到 `HeartbeatResponse.peer_recovery_states` 和 bootstrap peer 摘要里，包含 `next_probe_at`，便于 agent 和运维侧观察下一次恢复放行时间

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

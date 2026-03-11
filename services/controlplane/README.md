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
- `CONTROLPLANE_DIRECT_ATTEMPT_LEAD=150ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_WINDOW=600ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_BURST_INTERVAL=80ms`
- `CONTROLPLANE_DIRECT_ATTEMPT_RETENTION=2s`
- `CONTROLPLANE_DIRECT_ATTEMPT_MANUAL_RECOVER_AFTER=30s`

当前默认策略：

- relay 活跃链路优先触发 `relay_active`
- direct timeout / relay_kept 后进入冷却窗口，避免连续打洞
- 冷却结束后，如果 relay 仍持续活跃，则后续尝试会标记成 `manual_recover`

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

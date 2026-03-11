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

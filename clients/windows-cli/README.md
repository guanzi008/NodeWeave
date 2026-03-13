# Windows CLI

`windows-cli` 是当前 Windows 侧最小控制面接入客户端，负责：

- 设备注册
- 登录控制面
- 手动 heartbeat
- 拉取 bootstrap
- 查询节点列表

当前定位是 Windows 组网落地前的最小运维入口，用来先打通控制面接入和状态查询。

## 快速开始

```bash
go run ./cmd/windows-cli enroll
go run ./cmd/windows-cli status
go run ./cmd/windows-cli heartbeat --endpoints 203.0.113.10:51820
go run ./cmd/windows-cli bootstrap
go run ./cmd/windows-cli login
go run ./cmd/windows-cli nodes --token "$NODEWEAVE_ADMIN_TOKEN"
```

## 构建

```bash
make build
```

默认会生成：

- `dist/windows-cli.exe`

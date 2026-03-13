# Windows Agent

`windows-agent` 是当前 Windows 侧的首个常驻节点骨架，先把这些链路落地：

- 节点注册
- 周期 heartbeat
- bootstrap 同步
- Windows runtime snapshot 编译

当前定位是 Windows 组网接入第一阶段，先打通控制面和本地运行时文件，不在这一轮直接落 TUN/Wintun 数据面。

## 快速开始

```bash
go run ./cmd/windows-agent init-config
go run ./cmd/windows-agent enroll --config ~/.config/nodeweave/windows-agent.json
go run ./cmd/windows-agent run --config ~/.config/nodeweave/windows-agent.json
go run ./cmd/windows-agent status --config ~/.config/nodeweave/windows-agent.json
go run ./cmd/windows-agent bootstrap-status --config ~/.config/nodeweave/windows-agent.json
go run ./cmd/windows-agent runtime-status --config ~/.config/nodeweave/windows-agent.json
go run ./cmd/windows-agent heartbeat --config ~/.config/nodeweave/windows-agent.json
```

## 当前实现边界

- 当前会将 bootstrap 编译为 `windows-dry-run` overlay runtime snapshot
- 当前不会直接创建 Wintun、Windows 路由或 DNS
- `advertise_endpoints` 会作为静态 endpoint observation 上报控制面
- 控制面变更后会在 heartbeat/bootstrap 周期内刷新本地 runtime

## 构建

```bash
make build
```

默认会生成：

- `dist/windows-agent.exe`

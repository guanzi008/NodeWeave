# Linux CLI

Linux CLI 是当前第一个客户端子项目，用于验证节点注册、Bootstrap 拉取和心跳闭环。

## 运行

```bash
go run ./cmd/linux-cli enroll --server http://127.0.0.1:8080
go run ./cmd/linux-cli status
go run ./cmd/linux-cli heartbeat --endpoints 203.0.113.10:51820 --relay-region ap
```

管理员命令：

```bash
go run ./cmd/linux-cli login --server http://127.0.0.1:8080
go run ./cmd/linux-cli nodes --server http://127.0.0.1:8080 --token dev-admin-token
go run ./cmd/linux-cli routes --server http://127.0.0.1:8080 --token dev-admin-token
go run ./cmd/linux-cli route-create --server http://127.0.0.1:8080 --token dev-admin-token --network 10.20.0.0/16 --via-node node_xxx
go run ./cmd/linux-cli dns-zones --server http://127.0.0.1:8080 --token dev-admin-token
```

## 默认状态文件

`~/.config/nodeweave/linux-cli.json`

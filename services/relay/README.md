# Relay

`relay` 是最小 UDP 中继服务，负责为 `secure-udp` 数据面提供回退路径。

当前实现：

- 监听一个 UDP 地址
- 记录 `source_node_id -> UDP 源地址` 的短期映射
- 对带 `target_node_id` 的 `secure-udp` 报文做不解密转发
- `announce` 仅用于刷新映射，不做转发
- 当 peer 直连候选握手失败时，`secure-udp` 发送端会自动尝试 relay 候选
- 当 direct 再次恢复可用后，`secure-udp` 会重新切回 direct，relay 继续作为兜底路径

快速启动：

```bash
RELAY_ADDRESS=:3478 go run ./cmd/relay
```

环境变量：

- `RELAY_ADDRESS`：监听地址，默认 `:3478`
- `RELAY_MAPPING_TTL`：节点地址映射过期时间，默认 `2m`

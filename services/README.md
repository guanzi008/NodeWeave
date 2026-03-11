# Services

当前服务端子项目：

- `controlplane`：控制面 API、设备注册、节点管理、路由管理、DNS 区管理
- `relay`：最小 UDP 中继服务，给 `secure-udp` 数据面提供回退路径

后续可在此目录增加：

- `dns`
- `audit`
- `admin-api`

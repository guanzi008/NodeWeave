# 企业级异地组网系统架构设计

Version: 1.1  
Date: 2026-03-10  
Status: Draft for Implementation

## 1. 架构目标

本系统设计为一套企业级 Overlay Networking Platform，核心目标如下：

- 以统一控制面管理用户、设备、节点、证书、ACL、路由、DNS 与审计
- 以共享数据面实现跨平台加密隧道、NAT 穿透、Relay 回退
- 以多版本客户端覆盖桌面、移动、网关、CLI、嵌入式场景
- 以边界网关和桥接适配器实现与外部 VPN 网络互通

## 2. 设计原则

- 安全优先：控制面和数据面默认双向认证与最小权限。
- P2P 优先：优先走直连链路，失败再自动切换 Relay。
- 平台抽象：共享核心逻辑，平台差异通过 Adapter 层隔离。
- 配置收敛：所有运行时策略由控制面统一建模和下发。
- 可审计：重要行为、流量、策略命中必须留痕。
- 可扩展：Relay、DNS、审计存储、API 必须支持横向扩展。

## 3. 术语

| 术语 | 含义 |
| --- | --- |
| Control Plane | 节点注册、认证、配置、策略、审计管理的控制系统 |
| Data Plane | 隧道、路由、DNS、转发、出口节点等实际数据通路 |
| Node | 加入 Overlay 网络的逻辑节点 |
| Device | 与用户或站点关联的终端或网关实体 |
| Exit Node | 为其他节点提供默认外网出口的节点 |
| Relay | NAT 打洞失败时负责中继转发的节点 |
| Bridge Gateway | 用于连接外部网络或第三方 VPN 的边界网关 |

## 4. 总体架构

```text
                    ┌──────────────────────────────┐
                    │         Control Plane         │
                    │ Auth / ACL / Route / DNS /   │
                    │ Audit / Device / Config API  │
                    └──────────────┬───────────────┘
                                   │
                         Register / Sync / Heartbeat
                                   │
       ┌───────────────────────────┼───────────────────────────┐
       │                           │                           │
┌──────▼──────┐            ┌───────▼────────┐          ┌───────▼────────┐
│ Desktop App │            │ Mobile Client  │          │ Gateway / Edge │
│ Win/Mac/Linux│           │ Android / iOS  │          │ Linux/OpenWrt  │
└──────┬──────┘            └───────┬────────┘          └───────┬────────┘
       │                           │                           │
       └─────────────┬─────────────┴─────────────┬─────────────┘
                     │                           │
                 P2P Tunnel                 Relay Tunnel
                     │                           │
              ┌──────▼────────┐         ┌────────▼───────┐
              │ Overlay Engine │         │ Relay Cluster  │
              │ Route / DNS /  │         │ Region A/B/C   │
              │ Policy / Audit │         └────────────────┘
              └────────┬───────┘
                       │
             ┌─────────▼─────────┐
             │ External Networks │
             │ WG / ZT / LAN /   │
             │ Serial / USB / OT │
             └───────────────────┘
```

## 5. 核心模块划分

### 5.1 控制面

建议采用 Go 实现，原因是：

- 适合快速构建高并发 API 与 gRPC 服务
- 在后端生态中对 PostgreSQL、ClickHouse、Prometheus 支持成熟
- 便于快速交付管理后台与运维工具

控制面模块：

- `api-gateway`
- `identity-service`
- `device-service`
- `policy-service`
- `route-service`
- `dns-service`
- `audit-service`
- `signal-service`
- `admin-console`

### 5.2 数据面

建议采用 Rust 作为共享网络核心：

- 适合实现高性能用户态隧道、NAT 穿透和加密处理
- 便于通过 FFI 复用到 Windows、Linux、macOS、Android、iOS
- 相比 C/C++ 更利于控制内存安全

数据面模块：

- `overlay-core`
- `nat-engine`
- `relay-client`
- `route-manager`
- `dns-interceptor`
- `traffic-meter`
- `policy-enforcer`
- `device-forwarder`

### 5.3 平台壳层

| 平台 | 壳层职责 |
| --- | --- |
| Windows | Service、GUI、Wintun 适配、串口/USB 驱动交互 |
| Linux | systemd daemon、CLI、TUN/TAP、路由和 iptables/nftables 集成 |
| macOS | Network Extension、GUI、Keychain 集成 |
| Android | VpnService、前台服务、权限协调 |
| iOS | Packet Tunnel Provider、App 配置管理 |
| OpenWrt | 轻量 daemon、UCI 集成、dnsmasq / firewall 适配 |

## 6. 客户端版本策略

### 6.1 全功能版

模块：

- overlay-core
- nat-engine
- relay-client
- dns-interceptor
- route-manager
- policy-enforcer
- traffic-meter
- app-usage-agent
- usb-forwarder
- serial-forwarder
- gui-shell

### 6.2 核心版

模块：

- overlay-core
- nat-engine
- relay-client
- dns-interceptor
- route-manager
- policy-enforcer
- traffic-meter

### 6.3 网关桥接版

模块：

- overlay-core
- relay-client
- route-manager
- dns-service-lite
- bridge-adapter
- subnet-advertiser

### 6.4 CLI 最小版

模块：

- overlay-core
- nat-engine
- relay-client
- route-manager
- cli-shell

### 6.5 MCU 辅助版

模块：

- lightweight-signal
- serial-agent
- health-report

## 7. 控制面详细设计

### 7.1 身份认证

推荐模型：

- 用户身份：OIDC / SAML / 本地账号
- 设备身份：设备注册令牌 + 短期证书
- 节点身份：节点公私钥 + 控制面签发证书

认证流程：

1. 用户或设备发起注册。
2. 控制面校验租户、令牌或 SSO 身份。
3. 生成节点 ID、设备 ID、短期证书。
4. 返回初始配置、Bootstrap 节点、Relay 列表、DNS 和策略。

### 7.2 配置同步

使用 gRPC 双向流或长连接实现：

- 心跳上报
- NAT 观测数据回传
- 节点状态与质量指标上报
- 策略、路由、DNS 的增量更新

同步内容：

- 证书和密钥轮换计划
- 节点可见范围
- ACL 策略
- 路由表
- 内部 DNS zone
- 出口节点指派
- 审计采样策略

### 7.3 管理平面 API

管理 API 建议采用 REST，供后台、自动化脚本和 Terraform Provider 使用。

示例资源：

- `/api/v1/tenants`
- `/api/v1/users`
- `/api/v1/groups`
- `/api/v1/devices`
- `/api/v1/nodes`
- `/api/v1/policies`
- `/api/v1/routes`
- `/api/v1/dns/zones`
- `/api/v1/dns/records`
- `/api/v1/exit-nodes`
- `/api/v1/audit/events`
- `/api/v1/traffic/flows`

## 8. 数据面详细设计

### 8.1 隧道模型

基础隧道使用 WireGuard 风格的数据面模型：

- 每个节点拥有长期身份密钥
- 会话使用短期密钥派生
- 数据面通过 UDP 封装
- 多 endpoint 竞争，优先选择直连质量最优路径

实现方式不要求完整复刻 WireGuard 协议实现，但建议借鉴其密钥轮换和简洁报文模型。

### 8.2 虚拟网卡

| 平台 | 建议实现 |
| --- | --- |
| Windows | Wintun |
| Linux | TUN |
| macOS | Network Extension 虚拟接口 |
| Android | VpnService 虚拟接口 |
| iOS | Packet Tunnel 虚拟接口 |

### 8.3 路由管理

路由类型：

- 节点路由：`100.x.y.z/32` 或 IPv6 `/128`
- 子网路由：由网关节点发布
- 默认路由：由出口节点承载
- 策略路由：按用户、设备、应用、目的地址决定下一跳

冲突处理：

- 更精确前缀优先
- 同前缀按管理员优先级
- 再按节点健康度和地理偏好

### 8.4 NAT 穿透

实现步骤：

1. 节点向 STUN 服务探测外网地址与 NAT 类型。
2. 控制面交换双方可达 endpoint。
3. 双方同时向对端 endpoint 发起 UDP 握手。
4. 成功则转为 P2P。
5. 失败则切换到 Relay。

推荐支持的 NAT 信息：

- 公网 IP 和端口
- NAT 映射稳定性
- 近端 UDP 可达性
- IPv6 可用性
- 本地候选地址

注意事项：

- 对称 NAT 和企业防火墙下，P2P 成功率会下降，Relay 必须是正式能力而不是兜底实验功能。
- 在双栈网络中优先尝试 IPv6 直连。

### 8.5 Relay 中继

Relay 设计要求：

- 按地域部署，多区域路由
- 无状态接入，有状态度量
- 不解密业务内容，仅转发密文
- 支持带宽限速与租户隔离

调度策略：

- 同地域优先
- 最低 RTT 优先
- 次优 Relay 预热

### 8.6 链路选择

链路优先级：

1. IPv6 P2P
2. IPv4 P2P
3. 同地域 Relay
4. 跨地域 Relay

切换策略：

- 周期性进行路径探测
- 当前路径质量下降到阈值后切换
- 切换需保证会话尽量无感

## 9. Exit Node 设计

出口节点为其他节点提供默认路由转发，并承担外网 DNS 解析与策略审计。

关键能力：

- 支持用户级、设备级、组级出口策略
- 支持 `allow_lan` 控制是否保留本地局域网访问
- 支持流量审计和出口带宽限制
- 支持区域优选和故障切换

示例配置：

```yaml
exit_node:
  mode: enforced
  node_id: node_tokyo
  allow_lan: true
  allow_internet: true
  dns_mode: follow_exit
```

## 10. 子网路由与站点互联

站点互联通过网关节点实现，网关节点发布本地可达网段，控制面负责冲突检测与策略校验。

示例：

```yaml
routes:
  - network: 10.10.0.0/16
    via: node_beijing_gateway
    priority: 100

  - network: 172.16.20.0/24
    via: node_shanghai_gateway
    priority: 90
```

网关责任：

- 发布站点网段
- 承接来自 Overlay 的流量
- 将目标流量转发到本地 LAN 或工业网络
- 执行边界 ACL

## 11. DNS 体系设计

### 11.1 DNS 接管

客户端可开启 DNS 接管，将系统 DNS 请求重定向到 Overlay DNS。

支持模式：

- 全量接管
- 仅内部域名接管
- 跟随出口节点解析

实现方法：

- Windows：系统 DNS 配置 + WFP / 本地代理辅助
- Linux：`resolv.conf` 或 `systemd-resolved` 集成
- macOS / iOS：Network Extension DNS Settings
- Android：VpnService DNS 配置和域名路由

### 11.2 内部 DNS

内部 DNS 由控制面集中管理，可由专门的 `dns-service` 承载。

记录来源：

- 节点自动注册
- 管理后台录入
- API 创建
- 网关同步外部域名或静态记录

建议支持记录类型：

- A
- AAAA
- CNAME
- TXT
- SRV
- PTR

域名示例：

- `node1.internal.net`
- `camera.factory.net`
- `plc.factory.net`

### 11.3 DoH / DoT

外部访问内部 DNS 时可提供 DoH / DoT 入口，但核心控制仍建议基于自有内部 DNS 权威区。

## 12. ACL 与策略引擎

### 12.1 策略模型

策略维度：

- 用户
- 组
- 设备
- 节点
- 子网
- 端口
- 协议
- 应用
- 时间窗口

示例：

```yaml
policy:
  default_action: deny
  allow:
    - subject: group:dev
      destination: subnet:10.0.0.0/24
      ports: [22, 443]

  deny:
    - subject: user:guest
      destination: subnet:10.0.0.0/24
```

### 12.2 生效位置

- 控制面：负责策略编译与冲突检查
- 客户端：负责本地快速匹配和执行
- 网关：负责站点边界策略执行
- 出口节点：负责默认路由流量控制

### 12.3 应用策略

按平台能力实现：

- Windows：进程名、路径、签名信息
- Linux：进程名与 cgroup / net_cls 等能力结合
- Android：包名
- iOS：能力有限，更多依赖系统 extension 可见范围

示例：

```yaml
deny_apps:
  - steam.exe
  - com.game.xxx
```

## 13. 应用使用统计

### 13.1 数据项

- 应用名称
- 进程或包名
- 启动次数
- 会话次数
- 前台时长
- 后台时长
- 应用总网络流量
- 通过 VPN 的网络流量

### 13.2 平台差异

| 平台 | 可行性 | 说明 |
| --- | --- | --- |
| Windows | 高 | 可结合进程与流量映射 |
| Linux | 中 | 依赖发行版与权限能力 |
| macOS | 中 | 需评估系统 API 可见性 |
| Android | 中高 | 依赖 Usage Stats 与 VPN 统计 |
| iOS | 低 | 仅能提供较粗粒度或不支持全量应用统计 |

结论：

- 产品需求应将“应用使用统计”定义为分平台能力，而不是所有平台完全一致。
- 控制台需展示能力标签与数据可信度等级。

## 14. 网络流量审计

### 14.1 审计内容

- 租户 ID
- 用户 ID
- 设备 ID
- 节点 ID
- 源地址和端口
- 目标地址和端口
- 协议
- 起止时间
- 上传下载流量
- 命中策略
- 是否经过出口节点

### 14.2 数据管道

建议流程：

1. 客户端本地采样或聚合。
2. 通过批量上报接口发送到审计服务。
3. 审计服务写入 ClickHouse。
4. 异常事件写入告警管道。

### 14.3 告警

支持规则：

- 未授权访问尝试
- 高频跨网段扫描
- 短时间超大流量
- 命中高风险目标地址
- 敏感设备访问

## 15. USB / 串口转发设计

### 15.1 USB 转发

推荐实现：

- 优先评估 USB/IP
- 对 Windows 通过本地 Service 与驱动配合
- 对 Linux 网关支持内核或用户态桥接

风险：

- 驱动兼容复杂
- 实时性和稳定性受设备类型影响较大
- 不是所有 USB 设备都适合远程映射

### 15.2 串口转发

串口转发更适合作为第一阶段工业能力切入点。

模式：

- 本地串口封装为 TCP 流
- 远端恢复为虚拟 COM / tty
- 支持波特率、数据位、奇偶校验等参数同步

### 15.3 权限控制

- 设备独占锁
- 会话租约
- 审计连接开始和结束
- 按用户和设备授权

## 16. 外部 VPN 兼容策略

### 16.1 兼容边界

本系统与 WireGuard、Tailscale、ZeroTier、EasyTier 的兼容策略为“桥接互通”，不是“控制面合并”。

### 16.2 建议方案

| 外部网络 | 兼容方式 |
| --- | --- |
| WireGuard | 通过桥接网关建立 WG Peer，导入静态路由和 DNS |
| Tailscale | 通过子网路由器或出口节点互联，必要时使用 Tailscale API 同步部分节点信息 |
| ZeroTier | 通过边界节点桥接到目标网络，导入路由与访问策略 |
| EasyTier | 通过桥接节点建立站点互联和路由转发 |

### 16.3 不建议方案

- 试图让单个客户端直接同时成为多个控制面的原生节点
- 在产品早期追求所有第三方网络的节点自动发现与双向全量同步

## 17. API 概览

### 17.1 管理 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/auth/login` | 用户登录 |
| `POST` | `/api/v1/devices/register` | 设备注册 |
| `GET` | `/api/v1/nodes` | 节点列表 |
| `POST` | `/api/v1/nodes/{id}/approve` | 审批节点 |
| `GET` | `/api/v1/policies` | 查询策略 |
| `POST` | `/api/v1/policies` | 创建策略 |
| `GET` | `/api/v1/routes` | 查询路由 |
| `POST` | `/api/v1/routes` | 创建路由 |
| `GET` | `/api/v1/dns/zones` | 查询 DNS 区 |
| `POST` | `/api/v1/dns/records` | 创建 DNS 记录 |
| `GET` | `/api/v1/audit/events` | 查询审计事件 |
| `GET` | `/api/v1/traffic/flows` | 查询流量数据 |

### 17.2 节点信令 API

建议采用 gRPC：

- `Bootstrap`
- `Heartbeat`
- `WatchConfig`
- `ReportEndpoints`
- `ReportFlows`
- `ReportAppUsage`
- `RequestPeerConnect`

## 18. 数据存储设计

### 18.1 推荐技术选型

| 数据类型 | 存储 |
| --- | --- |
| 配置与元数据 | PostgreSQL |
| 流量与应用统计 | ClickHouse |
| 日志 | Loki |
| 指标 | Prometheus |
| 对象制品 | S3 兼容存储 |

### 18.2 核心表设计

#### PostgreSQL

`tenants`

| 字段 | 说明 |
| --- | --- |
| id | 租户 ID |
| name | 租户名称 |
| status | 状态 |
| created_at | 创建时间 |

`users`

| 字段 | 说明 |
| --- | --- |
| id | 用户 ID |
| tenant_id | 租户 ID |
| email | 邮箱 |
| display_name | 显示名 |
| status | 状态 |

`devices`

| 字段 | 说明 |
| --- | --- |
| id | 设备 ID |
| tenant_id | 租户 ID |
| user_id | 归属用户 |
| platform | 平台 |
| version | 客户端版本 |
| status | 状态 |

`nodes`

| 字段 | 说明 |
| --- | --- |
| id | 节点 ID |
| device_id | 设备 ID |
| overlay_ip | Overlay 地址 |
| public_key | 节点公钥 |
| relay_region | 当前中继区域 |
| last_seen_at | 最近心跳 |

`policies`

| 字段 | 说明 |
| --- | --- |
| id | 策略 ID |
| tenant_id | 租户 ID |
| name | 策略名 |
| action | allow/deny |
| spec_json | 规则 JSON |
| priority | 优先级 |

`routes`

| 字段 | 说明 |
| --- | --- |
| id | 路由 ID |
| tenant_id | 租户 ID |
| network_cidr | 网段 |
| via_node_id | 下一跳节点 |
| priority | 优先级 |
| status | 状态 |

`dns_zones`

| 字段 | 说明 |
| --- | --- |
| id | 区 ID |
| tenant_id | 租户 ID |
| name | 域名 |
| type | internal/external-bridge |

`dns_records`

| 字段 | 说明 |
| --- | --- |
| id | 记录 ID |
| zone_id | 所属域 |
| name | 记录名 |
| type | 记录类型 |
| value | 记录值 |
| ttl | TTL |

#### ClickHouse

`traffic_flows`

分区建议：

- 按日期分区
- 按租户和节点主键排序

字段：

- ts_start
- ts_end
- tenant_id
- user_id
- device_id
- node_id
- src_ip
- dst_ip
- protocol
- bytes_up
- bytes_down
- policy_id
- exit_node_id

`app_usage_samples`

字段：

- sample_time
- tenant_id
- user_id
- device_id
- app_name
- app_id
- foreground_seconds
- background_seconds
- launches
- sessions
- vpn_bytes

## 19. 安全设计

### 19.1 密钥与证书

- 节点长期身份密钥在本地安全存储
- 控制面签发短期证书
- 支持定期轮换
- 支持节点吊销与 CRL / OCSP 风格状态检查

### 19.2 管理安全

- 管理后台必须支持 MFA
- 敏感操作需要审计与审批
- API Token 应区分只读与写权限

### 19.3 数据保护

- 审计数据传输加密
- 存储层启用磁盘或表级加密
- 日志与审计分级脱敏

## 20. 可观测性与运维

### 20.1 指标

控制面指标：

- API 延迟
- 配置下发成功率
- 在线节点数
- 心跳延迟
- 策略编译耗时

数据面指标：

- P2P 建链成功率
- Relay 占比
- 隧道 RTT
- 数据面丢包率
- 出口节点带宽

### 20.2 日志

日志要求：

- 结构化 JSON
- 统一 trace_id / tenant_id / node_id
- 分类为 audit、system、security、network

### 20.3 SLO

- 控制面月可用性：99.95%
- Relay 服务月可用性：99.9%
- 节点配置同步延迟 P95 小于 3 秒

## 21. 部署拓扑建议

### 21.1 小规模部署

- 1 套控制面
- 1 个 PostgreSQL 主实例
- 1 个 ClickHouse 单节点
- 2 个 Relay 节点

### 21.2 生产部署

- 控制面服务多实例
- PostgreSQL 主从或高可用集群
- ClickHouse 集群
- 多地域 Relay
- 独立 DNS 服务集群
- 独立审计与告警管道

## 22. 研发顺序建议

### 阶段 1

- 控制面基础服务
- Linux CLI / daemon
- Windows 全功能版基础
- 隧道、NAT、Relay
- 基础 ACL、路由、内部 DNS

### 阶段 2

- Android / iOS 核心版
- Exit Node
- DNS 接管
- 完整审计链路
- 网关桥接版

### 阶段 3

- USB / 串口
- 应用使用统计
- 多租户增强
- 第三方网络桥接增强

## 23. 关键风险

- NAT 穿透成功率受真实网络环境影响大，必须将 Relay 成本纳入正式容量规划。
- iOS 应用统计和细粒度应用策略能力受系统限制，需求侧需接受能力差异。
- USB 远程映射的设备兼容性不可一次性覆盖，应优先聚焦少数高价值设备类别。
- 第三方 VPN 互通若试图做成“协议级兼容”，复杂度会远超预期，应坚持桥接策略。

## 24. 推荐的首版落地边界

首版只承诺以下闭环：

- 用户注册并批准设备
- 节点自动获取 Overlay IP
- P2P 优先，失败走 Relay
- 内部域名解析
- 基础 ACL
- 基础站点路由
- Windows / Linux 可用

这条边界能最大化降低实现风险，同时保留向移动端、工业场景和企业高级治理能力扩展的空间。

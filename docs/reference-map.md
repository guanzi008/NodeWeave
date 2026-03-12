# 需求追踪与开源借鉴矩阵

Version: 1.0  
Date: 2026-03-10  
Status: Active Baseline

## 1. 文档目的

这份文档用于约束后续实现不要偏离最初 PRD 和架构文档，并明确各能力模块应借鉴哪些开源项目、借鉴到什么边界、当前仓库已经做到哪里。

它不是“抄协议清单”，而是“实现参考基线”。

## 2. 基本原则

- 以 [PRD](prd.md) 为需求边界，以 [架构文档](architecture.md) 为实现边界。
- “兼容 WireGuard / Tailscale / ZeroTier / EasyTier”始终定义为桥接互通，不等于复用对方控制面。
- 借鉴重点是设计模式、链路策略、部署模型和工程经验，不是逐字复刻协议实现。
- 当前代码虽然是 Linux-first，但产品目标始终是多平台、多版本、企业级控制与审计，不允许开发方向长期收缩成单平台工具。

## 3. 开源借鉴总表

| 能力域 | 最初文档目标 | 主要借鉴项目 | 借鉴重点 | 明确不做 |
| --- | --- | --- | --- | --- |
| 数据面基础隧道 | 长期身份、UDP 封装、密钥轮换 | WireGuard | 简洁报文模型、密钥派生、会话切换 | 不承诺协议级 WireGuard 完全兼容 |
| NAT 打洞 | STUN、endpoint 交换、直连优先、控制面协同 simultaneous-open | Tailscale、EasyTier、Nebula | endpoint 候选管理、NAT 摘要、peer transport 摘要、协调直连窗口、失败回退 | 不假设所有 NAT 都能直连 |
| Relay | NAT 失败时中继且不解密业务数据 | Tailscale DERP 思路、Nebula | 区域化中继、透明转发、路径切换 | 不把 Relay 设计成明文代理 |
| Overlay 网络 | 节点互联、站点互联、子网路由 | ZeroTier、EasyTier、tinc | mesh/route 抽象、站点间路由发布 | 不做无限制 L2 广播桥接 |
| 控制面收敛 | 注册、心跳、配置同步、节点管理 | Tailscale、Nebula | 控制面建模、节点状态同步、增量配置 | 不接管第三方控制面 |
| 内部 DNS | 节点命名、服务发现、内部域名 | Tailscale MagicDNS 思路 | 节点自动命名、内部域名收敛 | 不把外部公共 DNS 逻辑混进控制面核心 |
| Exit Node | 默认出口、DNS 跟随出口 | Tailscale Exit Node | 默认路由切换、出口策略 | 不做透明商用代理平台 |
| 网关桥接 | 外部 VPN / 站点网络互联 | WireGuard、Tailscale Subnet Router、ZeroTier、EasyTier | 子网发布、边界路由、DNS 导入 | 不做协议级混控 |
| ACL / 策略 | 用户、设备、节点、子网、应用多维度策略 | Tailscale ACL、Nebula policy | 统一策略模型、默认拒绝、命中追踪 | 不做无法审计的隐式放行 |
| 流量审计 | 连接级/节点级/用户级流量归集 | 企业网关产品经验、Prometheus/Loki/ClickHouse 生态 | 指标、日志、审计分层 | 不做 DPI 内容审查 |
| USB / 串口转发 | 工控设备远程接入 | usbip、libusb | 设备枚举、转发、独占控制 | 不在第一阶段实现全平台驱动兼容 |

## 4. 需求到仓库的当前映射

| 需求主题 | 目标状态 | 当前仓库状态 | 主要代码位置 | 后续仍需参考 |
| --- | --- | --- | --- | --- |
| 控制面 MVP | 节点注册、心跳、bootstrap、路由、DNS zone | 已有基础实现 | `services/controlplane` | Tailscale / Nebula 的配置同步思路 |
| Relay | 区域中继、opaque forwarding | 已有最小实现 | `services/relay` | DERP 式调度、区域扩展 |
| Linux agent | 注册、同步、heartbeat、runtime apply | 已有基础实现 | `clients/linux-agent` | 客户端状态机和恢复策略 |
| secure transport | 加密 UDP、握手、回退、恢复 | 已有基础实现 | `packages/runtime/go/secureudp` | WireGuard 式会话和重键思路 |
| 会话候选 / probe | direct/relay candidate、可达性选择 | 已有基础实现 | `packages/runtime/go/session` | Tailscale / EasyTier NAT 探测策略 |
| STUN 外网发现 | reflexive endpoint 上报控制面 | 已有基础实现 | `packages/runtime/go/stun`、`clients/linux-agent` | Tailscale / Nebula 的 endpoint 发现与刷新策略 |
| Endpoint freshness | endpoint 来源、观测时间、bootstrap 下发 | 已有基础实现 | `packages/contracts/go/api`、`services/controlplane/internal/store`、`packages/runtime/go/session`、`clients/linux-agent` | Tailscale / EasyTier 的 endpoint 管理与排序策略 |
| Direct warmup / 协同打洞 | 业务流量前主动做 direct 握手预热，控制面下发短期 direct attempt | 已有基础实现 | `packages/runtime/go/secureudp`、`clients/linux-agent`、`services/controlplane/internal/store` | Tailscale / Nebula 的 endpoint warmup / simultaneous open 思路 |
| dataplane routing | route -> peer -> candidate 编译 | 已有基础实现 | `packages/runtime/go/dataplane` | ZeroTier / EasyTier 的路径收敛经验 |
| TUN / packet pump | Linux TUN 接入 | 已有骨架 | `packages/runtime/go/tunnel` | Linux 网络栈幂等编排 |
| Exit Node | 默认出口与 DNS 跟随 | 只有基础模型和 plan | `packages/runtime/go/plan/linux`、`services/controlplane` | Tailscale Exit Node 体验 |
| 内部 DNS | 节点命名、服务发现、权威解析 | 只有模型，未成品化 | `services/controlplane`、文档 | MagicDNS 风格命名与缓存策略 |
| ACL / 审计 | 企业级策略、命中追踪、流量审计 | 仍未落地成主链路 | 文档和任务拆分为主 | Tailscale ACL / Nebula policy |
| 网关桥接 | WireGuard / ZeroTier / EasyTier / Tailscale 互通 | 还未开始 | `gateways` 目录仍为空壳 | 各项目的边界网关能力 |
| USB / 串口 | 远程设备转发 | 未开始 | 文档与任务拆分 | usbip、libusb |
| Windows/macOS/mobile | 多平台客户端 | 未开始 | `mobile` 和其他平台目录仍为空壳 | 平台壳层和系统网络扩展 |

## 5. 后续开发时必须持续遵守的约束

### 5.1 数据面

- 继续保持“P2P 优先，Relay 必须正式可用”的原则。
- 新增 secure transport 能力时，优先借鉴 WireGuard 的简洁会话模型，而不是引入复杂多层协议栈。
- NAT 穿透不能只停留在 probe 和 candidate 选择，必须继续向 STUN、endpoint 交换、simultaneous open 演进。

### 5.2 控制面

- 不允许为了当前 Linux MVP 简化掉多租户、ACL、审计、内部 DNS 这些最初 P0/P1 能力的架构位置。
- 后续接口设计必须继续兼容管理面和自动化面，而不是只服务当前 CLI。

### 5.3 兼容策略

- 与外部网络的“兼容”继续按桥接互通实现。
- 不将第三方协议细节强耦合进主控制面模型。
- 先做边界网关和路由/DNS 导入导出，再讨论更深层协议适配。

### 5.4 工业场景

- USB / 串口转发不能从主路线图中消失，它是最初文档的明确目标，不是可选附加功能。
- 相关实现优先从 Linux 网关和 Linux 桌面端落地，再扩平台。

## 6. 推荐的实现优先顺序

1. 完成真实 NAT 穿透链路：STUN、endpoint 交换、直连打洞。
2. 强化 secure transport：重键、会话恢复、状态指标。
3. 补齐控制面：增量配置流、ACL 编译、内部 DNS、Exit Node 真闭环。
4. 开始网关桥接：优先 WireGuard 与 Tailscale 子网路由互通。
5. 开始工业能力：串口转发先行，USB 转发后跟。
6. 扩展平台：Windows、OpenWrt、Android、iOS。

## 7. 结论

后续开发以三份文档共同约束：

- [PRD](prd.md)
- [架构文档](architecture.md)
- [需求追踪与开源借鉴矩阵](reference-map.md)

如果后续某项实现明显偏离这三份文档，应先修正文档或显式记录偏差原因，再继续开发。

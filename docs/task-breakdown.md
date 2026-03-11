# 企业级异地组网系统开发任务拆分

Version: 1.0  
Date: 2026-03-10

## 1. 使用方式

本文档按 Epic > Story > Task 的粒度组织，可直接映射到 Jira、GitHub Issues 或线性迭代计划。

建议标签：

- `epic`
- `backend`
- `client`
- `network`
- `mobile`
- `gateway`
- `security`
- `audit`
- `dns`
- `route`

## 2. 里程碑

| 里程碑 | 时间建议 | 验收目标 |
| --- | --- | --- |
| M1 | 第 1 到 4 周 | 控制面骨架、节点注册、Linux CLI 可连通 |
| M2 | 第 5 到 8 周 | P2P / Relay、Windows 客户端、基础 ACL 和 DNS |
| M3 | 第 9 到 12 周 | MVP 闭环上线，支持基础站点互联 |
| M4 | 第 13 到 20 周 | 移动端、Exit Node、完整审计、网关版 |
| M5 | 第 21 到 28 周 | USB / 串口、应用统计、多租户增强 |

## 3. Epic A：平台基础与工程化

### Story A1：仓库与工程基线

- Task A1-1：建立 monorepo 目录结构
- Task A1-2：定义后端、客户端、移动端、网关端代码边界
- Task A1-3：接入 CI，至少包含 lint、unit test、build
- Task A1-4：建立版本号与制品发布规范
- Task A1-5：建立基础文档目录和 ADR 机制

### Story A2：共享协议与配置模型

- Task A2-1：定义节点、设备、策略、路由、DNS 的 protobuf 或 JSON schema
- Task A2-2：定义配置版本号与增量同步格式
- Task A2-3：定义审计事件和流量上报 schema
- Task A2-4：定义客户端本地配置文件格式

## 4. Epic B：控制面基础服务

### Story B1：认证与设备注册

- Task B1-1：实现本地账号登录 API
- Task B1-2：预留 OIDC 接口
- Task B1-3：实现设备注册令牌机制
- Task B1-4：实现节点审批与吊销流程
- Task B1-5：实现节点证书签发与轮换

### Story B2：节点管理

- Task B2-1：实现节点列表查询 API
- Task B2-2：实现节点在线状态聚合
- Task B2-3：实现设备与节点关系模型
- Task B2-4：实现心跳接收和状态更新

### Story B3：配置下发

- Task B3-1：实现 gRPC `Bootstrap`
- Task B3-2：实现 gRPC `WatchConfig`
- Task B3-3：实现配置版本对比与增量下发
- Task B3-4：实现失败回滚机制

## 5. Epic C：Overlay 网络核心

### Story C1：基础隧道

- Task C1-1：实现虚拟网卡抽象接口
- Task C1-2：实现密钥管理与会话协商
- Task C1-3：实现基础数据面收发
- Task C1-4：实现 Linux TUN 适配
- Task C1-5：实现 Windows Wintun 适配

### Story C2：NAT 穿透

- Task C2-1：实现 STUN 客户端
- Task C2-2：实现 endpoint 探测与排序
- Task C2-3：实现 UDP simultaneous open
- Task C2-4：实现 NAT 类型采样指标
- Task C2-5：实现 P2P 失败后的 Relay 回退

### Story C3：链路治理

- Task C3-1：实现链路质量打分
- Task C3-2：实现 P2P / Relay 动态切换
- Task C3-3：实现 IPv6 优先策略
- Task C3-4：实现会话无感切换验证

## 6. Epic D：Relay 系统

### Story D1：Relay 服务

- Task D1-1：实现单区域 Relay 服务
- Task D1-2：实现密文中继能力
- Task D1-3：实现租户级基础限流
- Task D1-4：输出 Relay 带宽与会话指标

### Story D2：Relay 调度

- Task D2-1：实现 Relay 区域注册表
- Task D2-2：实现客户端 Relay 优选逻辑
- Task D2-3：实现同地域优先与 RTT 选择
- Task D2-4：实现 Relay 故障摘除

## 7. Epic E：路由与 Exit Node

### Story E1：基础路由

- Task E1-1：实现节点路由模型
- Task E1-2：实现子网路由模型
- Task E1-3：实现路由冲突检测
- Task E1-4：实现客户端路由编程

### Story E2：Exit Node

- Task E2-1：实现出口节点标记与分配
- Task E2-2：实现默认路由下发
- Task E2-3：实现 `allow_lan` 行为
- Task E2-4：实现出口节点健康检查和切换
- Task E2-5：实现出口节点流量审计

## 8. Epic F：DNS 系统

### Story F1：内部 DNS 服务

- Task F1-1：实现 DNS zone 与 record 存储
- Task F1-2：实现权威解析服务
- Task F1-3：实现节点自动注册记录
- Task F1-4：实现 API 管理 DNS 记录

### Story F2：DNS 接管

- Task F2-1：实现 Windows DNS 接管
- Task F2-2：实现 Linux DNS 接管
- Task F2-3：实现 Android / iOS DNS 策略接入
- Task F2-4：实现跟随出口节点的 DNS 模式

## 9. Epic G：ACL 与策略控制

### Story G1：策略模型

- Task G1-1：定义 allow / deny 规则模型
- Task G1-2：定义用户、设备、节点、应用维度选择器
- Task G1-3：定义规则优先级和冲突解决逻辑

### Story G2：策略执行

- Task G2-1：实现控制面策略编译
- Task G2-2：实现客户端本地匹配器
- Task G2-3：实现网关边界策略执行
- Task G2-4：实现策略命中上报

### Story G3：应用策略

- Task G3-1：实现 Windows 进程识别
- Task G3-2：实现 Android 包名识别
- Task G3-3：评估 Linux / macOS / iOS 可行边界
- Task G3-4：实现禁止应用通过 VPN 的最小闭环

## 10. Epic H：审计与可观测性

### Story H1：流量审计

- Task H1-1：定义流量上报格式
- Task H1-2：实现客户端采样聚合
- Task H1-3：实现 ClickHouse 写入链路
- Task H1-4：实现流量查询 API

### Story H2：安全审计

- Task H2-1：记录登录、注册、审批、吊销事件
- Task H2-2：记录策略变更事件
- Task H2-3：记录敏感设备连接事件
- Task H2-4：实现基础告警规则

### Story H3：指标与日志

- Task H3-1：接入 Prometheus 指标
- Task H3-2：输出结构化日志
- Task H3-3：建立基础 Grafana Dashboard

## 11. Epic I：桌面客户端

### Story I1：Linux 客户端

- Task I1-1：实现 daemon
- Task I1-2：实现 CLI 登录与入网
- Task I1-3：实现 systemd 服务
- Task I1-4：实现状态查询命令

### Story I2：Windows 客户端

- Task I2-1：实现 Windows Service
- Task I2-2：实现 GUI 登录与设备状态页
- Task I2-3：实现 Wintun 管理
- Task I2-4：实现自动启动与升级机制

### Story I3：macOS 客户端

- Task I3-1：实现 Network Extension 骨架
- Task I3-2：实现 GUI 配置页
- Task I3-3：实现证书和 Keychain 集成

## 12. Epic J：移动客户端

### Story J1：Android

- Task J1-1：实现 VpnService 骨架
- Task J1-2：实现登录、入网、状态展示
- Task J1-3：实现 DNS 与出口节点配置
- Task J1-4：实现应用包名统计可行性版本

### Story J2：iOS

- Task J2-1：实现 Packet Tunnel Provider 骨架
- Task J2-2：实现 App 配置和设备注册
- Task J2-3：实现 DNS / 路由基础能力
- Task J2-4：明确定义应用统计不支持或降级路径

## 13. Epic K：网关桥接版

### Story K1：OpenWrt / Linux 网关

- Task K1-1：实现轻量 daemon
- Task K1-2：实现子网发布
- Task K1-3：实现 dnsmasq 集成
- Task K1-4：实现 firewall / nftables 集成

### Story K2：外部网络桥接

- Task K2-1：实现 WireGuard 网关桥接
- Task K2-2：实现 EasyTier 网关桥接
- Task K2-3：验证 Tailscale 子网路由器互通方案
- Task K2-4：验证 ZeroTier 边界桥接方案

## 14. Epic L：USB / 串口转发

### Story L1：串口转发

- Task L1-1：实现串口采集与 TCP 封装
- Task L1-2：实现远端虚拟串口恢复
- Task L1-3：实现串口参数同步
- Task L1-4：实现独占锁和审计

### Story L2：USB 转发

- Task L2-1：评估 USB/IP 技术路线
- Task L2-2：实现 Linux 网关端原型
- Task L2-3：实现 Windows 端原型
- Task L2-4：建立设备兼容矩阵

## 15. Epic M：管理后台

### Story M1：基础控制台

- Task M1-1：实现登录与租户切换
- Task M1-2：实现用户、设备、节点列表
- Task M1-3：实现审批与吊销操作

### Story M2：网络管理

- Task M2-1：实现路由管理页
- Task M2-2：实现 DNS 管理页
- Task M2-3：实现 Exit Node 管理页
- Task M2-4：实现 Relay 区域配置页

### Story M3：审计与报表

- Task M3-1：实现流量审计查询
- Task M3-2：实现应用使用统计看板
- Task M3-3：实现告警事件看板

## 16. Epic N：测试与验证

### Story N1：自动化测试

- Task N1-1：为控制面建立 API 单测
- Task N1-2：为策略编译建立单测
- Task N1-3：为路由冲突检测建立单测
- Task N1-4：为 DNS 解析建立单测

### Story N2：网络实验室

- Task N2-1：建立多 NAT 场景测试环境
- Task N2-2：建立 IPv4 / IPv6 双栈测试
- Task N2-3：建立跨地域 Relay 压测
- Task N2-4：建立 OpenWrt 网关验证环境

### Story N3：兼容性测试

- Task N3-1：验证 Windows 10/11
- Task N3-2：验证主流 Linux 发行版
- Task N3-3：验证 Android 主流版本
- Task N3-4：验证 iOS 与 macOS Network Extension 边界

## 17. MVP 必做清单

MVP 上线前必须完成：

- 节点注册与证书签发
- Linux CLI 入网
- Windows 基础客户端入网
- P2P / Relay 双链路
- 基础 ACL
- 内部 DNS
- 基础路由
- 流量审计最小链路
- 管理后台最小可操作页面

## 18. 建议团队编制

建议最小团队：

- 后端 2 到 3 人
- 网络核心 2 人
- Windows / 桌面客户端 1 到 2 人
- 移动端 2 人
- 网关 / OpenWrt 1 人
- 前端 / 管理后台 1 人
- QA 1 到 2 人
- SRE / DevOps 1 人

## 19. 关键依赖

- STUN / Relay 基础设施
- PostgreSQL、ClickHouse、Prometheus、Loki
- Windows 驱动与 Wintun
- Apple Developer 能力与 Network Extension 签名流程
- Android 企业分发或应用商店策略

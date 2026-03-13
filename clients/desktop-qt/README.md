# Desktop Qt Client

`desktop-qt` 是当前图形客户端子项目，使用：

- `C++17`
- `Qt6` 优先，自动兼容 `Qt5`
- `CMake`

当前先落最小可用桌面壳，已经具备：

- 控制面地址配置
- 健康检查
- 管理员登录
- 节点列表查询
- 设备注册
- 串口转发映射编辑
- USB 转发映射编辑
- Linux / Windows agent forwarding snippet 导出
- Linux / Windows agent forwarding snippet 导入
- 串口 / USB forwarding report 本地查看
- 本地 `QSettings` 持久化

这版定位是跨平台 GUI 基线，后续再继续接：

- 节点 bootstrap / runtime 展示
- Linux / Windows agent 管理
- 串口 / USB 实时会话管理
- 路由、DNS、Relay、Exit Node 可视化

## 构建

Qt6 优先，Qt5 兼容：

```bash
cmake -S clients/desktop-qt -B build/desktop-qt
cmake --build build/desktop-qt -j
```

生成产物：

- `build/desktop-qt/nodeweave-desktop`

## 依赖

需要以下 Qt 组件：

- `Qt::Widgets`
- `Qt::Network`

## 当前实现边界

- 当前只接控制面 REST API，不直接接 secure-udp 数据面
- 当前没有 `.ui` 文件，界面全由 C++ 代码构建
- 串口 / USB 标签页当前编辑的是 forwarding 配置、agent snippet 导入导出和本地 report 查看，不直接枚举本机硬件
- 当前没有打包脚本，先保证开发态构建和运行

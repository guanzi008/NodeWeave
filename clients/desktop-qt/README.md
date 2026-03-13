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
- 本机串口自动扫描与回填
- 本机 USB 自动扫描与回填
- 驱动提示与内置规则建议
- Linux / Windows agent forwarding snippet 导出
- Linux / Windows agent forwarding snippet 导入
- 串口 / USB forwarding report 本地查看
- 图形界面中文化
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

- `build/desktop-qt/nodeweave`
- `build/desktop-qt/nodeweave.desktop`

## Linux / DDE 任务栏图标

当前已经补齐：

- Qt 应用内窗口图标
- `QApplication::setDesktopFileName("nodeweave")`
- `StartupWMClass=nodeweave`
- 标准 Linux `.desktop` 入口
- `hicolor` 图标安装路径

安装到当前用户目录：

```bash
cmake --install build/desktop-qt --prefix ~/.local
```

安装后 DDE / 其它 Linux 桌面环境会通过安装后的 `nodeweave.desktop`、
`nodeweave` 图标名和 `WM_CLASS` 识别任务栏图标与启动器归属。

## 依赖

需要以下 Qt 组件：

- `Qt::Widgets`
- `Qt::Network`

## 当前实现边界

- 当前只接控制面 REST API，不直接接 secure-udp 数据面
- 当前没有 `.ui` 文件，界面全由 C++ 代码构建
- 串口标签页会自动扫描本机串口设备，并根据当前驱动给出中文规则建议
- USB 标签页会自动扫描本机 USB 设备，并显示驱动/类别对应的转发建议
- 当前导出的 Linux agent forwarding snippet 已可驱动真实串口转发和基于 usbip 的真实 USB 转发
- Windows agent snippet 目前仍是配置和报告链路，不会直接打开 Windows 串口或 USB 设备
- 当前串口 / USB 规则是内置启发式规则，不等同于完整驱动兼容矩阵
- 当前没有打包脚本，先保证开发态构建和运行

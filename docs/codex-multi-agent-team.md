# Codex Multi-Agent Team

本文档包含两种方案：

- API key 版：OpenAI Agents SDK + Codex tool
- 账号登录版：Python 调度器 + `codex exec`

## API Key 版

这个脚本使用 OpenAI Agents SDK 作为编排层，使用 Codex tool 作为实际仓库执行器，把一个开发任务拆给四个角色：

- `Planner`
- `Implementer`
- `Tester`
- `Reviewer`

顶层 `DevelopmentManager` 会按固定顺序调度这些角色：

1. 规划
2. 实现
3. 验证
4. 评审

如果验证或评审发现阻塞问题，管理器会把问题回退给实现角色，再执行一轮验证和评审。

## 文件

- [codex_multi_agent_team.py](/home/hao/AAA/NodeWeave/scripts/codex_multi_agent_team.py)
- [requirements-codex-agents.txt](/home/hao/AAA/NodeWeave/scripts/requirements-codex-agents.txt)
- [codex_cli_multi_agent_team.py](/home/hao/AAA/NodeWeave/scripts/codex_cli_multi_agent_team.py)

## 准备

```bash
cd /home/hao/AAA/NodeWeave
python3 -m venv .venv
source .venv/bin/activate
pip install -r scripts/requirements-codex-agents.txt
export OPENAI_API_KEY=YOUR_KEY
```

如果你使用 Codex 专用 key，也可以设置：

```bash
export CODEX_API_KEY=YOUR_KEY
```

## 运行

```bash
python3 scripts/codex_multi_agent_team.py \
  --workspace /home/hao/AAA/NodeWeave \
  --task "Implement a new relay health-check endpoint and add tests"
```

## 常用参数

- `--manager-model`：顶层调度 agent 模型
- `--codex-model`：Codex tool 内部线程模型
- `--reasoning-effort`：Codex reasoning effort
- `--approval-policy`：Codex approval policy
- `--network-access`：允许 Codex 访问网络
- `--web-search-mode`：控制 Codex web search
- `--skip-git-repo-check`：如果工作区不是 git 仓库，需要开启

当前 `/home/hao/AAA/NodeWeave` 不在 git 仓库里，脚本会自动跳过 git 检查；你也可以显式传入 `--skip-git-repo-check`。

## 账号登录版

如果你没有 API key，而是使用账号级 Codex 登录，请改用本地 CLI 编排器。

先登录：

```bash
/home/hao/.vscode/extensions/openai.chatgpt-26.304.20706-linux-x64/bin/linux-x86_64/codex login --device-auth
```

再运行：

```bash
python3 scripts/codex_cli_multi_agent_team.py \
  --dangerously-bypass-sandbox \
  --skip-planner \
  --history-limit 6 \
  --worker-timeout-seconds 240 \
  --workspace /home/hao/AAA/NodeWeave \
  --task "Implement a new relay health-check endpoint and add tests"
```

这个版本直接调用本机 `codex exec`，复用 CLI 登录态，不依赖 `OPENAI_API_KEY` 或 `CODEX_API_KEY`。

如果 `codex` 不在 PATH，脚本会自动扫描常见 VS Code 扩展目录；你也可以显式指定：

```bash
python3 scripts/codex_cli_multi_agent_team.py \
  --dangerously-bypass-sandbox \
  --skip-planner \
  --history-limit 6 \
  --worker-timeout-seconds 240 \
  --codex-bin /home/hao/.vscode/extensions/openai.chatgpt-26.304.20706-linux-x64/bin/linux-x86_64/codex \
  --workspace /home/hao/AAA/NodeWeave \
  --task "Implement a new relay health-check endpoint and add tests"
```

如果 worker 日志里出现：

```text
Sandbox(LandlockRestrict)
```

说明不是任务本身失败，而是 Codex 自带 sandbox 在当前宿主机里起不来。此时需要加 `--dangerously-bypass-sandbox`。

如果 planner 在大仓库里收集上下文太慢，可以直接开启：

- `--skip-planner`：跳过 planner，直接从 implementer 开始
- `--worker-timeout-seconds 240`：给每个 worker 加超时，避免一轮 run 无限制挂起
- `--history-limit 6`：自动注入最近且最相似的历史 run，上下文会写到 `history_context.txt`
- `--history-max-chars 12000`：限制注入给 worker 的历史摘要长度

## 如何看进度

脚本启动后会立刻打印：

- `Run directory`
- `current_phase.txt` 的路径
- `latest` 链接
- `history_context.txt` 的路径和命中的历史 run 数量

你可以直接看当前阶段：

```bash
cat /home/hao/AAA/NodeWeave/.codex-team-runs/latest/current_phase.txt
```

看某个 worker 的实时日志：

```bash
tail -f /home/hao/AAA/NodeWeave/.codex-team-runs/latest/planner.stdout.log
tail -f /home/hao/AAA/NodeWeave/.codex-team-runs/latest/implementer.stdout.log
tail -f /home/hao/AAA/NodeWeave/.codex-team-runs/latest/tester.stdout.log
tail -f /home/hao/AAA/NodeWeave/.codex-team-runs/latest/reviewer.stdout.log
```

历史上下文和结构化索引在这里：

```bash
cat /home/hao/AAA/NodeWeave/.codex-team-runs/latest/history_context.txt
cat /home/hao/AAA/NodeWeave/.codex-team-runs/latest/history_runs.json
cat /home/hao/AAA/NodeWeave/.codex-team-runs/latest/run_manifest.json
```

如果你想直接在当前终端看到 worker 输出，可以加：

```bash
python3 scripts/codex_cli_multi_agent_team.py \
  --stream \
  --dangerously-bypass-sandbox \
  --skip-planner \
  --history-limit 6 \
  --worker-timeout-seconds 240 \
  --workspace /home/hao/AAA/NodeWeave \
  --task "Implement a new relay health-check endpoint and add tests"
```

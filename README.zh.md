# Mainline

[![CI](https://github.com/mainline-org/mainline/actions/workflows/ci.yml/badge.svg)](https://github.com/mainline-org/mainline/actions/workflows/ci.yml)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: layered](https://img.shields.io/badge/License-Apache--2.0%20%2F%20CC--BY--4.0%20%2F%20Commercial-blue.svg)](#授权)

- 官网：https://mainline.sh
- Hosted Hub：https://mainline.sh/hub/
- 详细文档：[docs/reference.zh.md](./docs/reference.zh.md)
- English version: [README.md](./README.md)

**AI 时代的 Git。**

Mainline 让 Agent 自动把开发者的意图和决策随着代码一起保存到 Git 仓库。

代码的历史已经由 Git 保存。Mainline 做的是把 Agent 每一次重要工程判断也放进
同一个协作底座：原始目标、思考路径、关键决策、取舍、验证、明确约束、已经放弃
的路线，以及这次判断最终对应哪个 commit。

团队不需要保存一堆 AI 对话，也不需要发明单独的共享记忆服务。Agent 从 Git 里
读取历史判断，完成工作后再把新的判断写回 Git。

<img width="1600" alt="Mainline Hub 展示一条 sealed engineering intent" src="https://github.com/user-attachments/assets/71bd98d0-64db-4f41-86eb-342dbafbdfc3" />

## 为什么需要它

Agent 写代码很快，但只看代码，很容易看不到历史。Mainline 不是另一个给人维护的知识库，而是 Agent 像用 Git 一样自动读写的工具。

### 不再重复已经放弃的路线

已经尝试并最终放弃的方案，往往不会体现在代码里。

比如团队试过用 Redis queue 处理 billing events，后来因为 replication lag 导致重复
扣费而放弃。下一个 Agent grep 到半成品 TODO 时，很可能觉得“补完就好”。Mainline
会在它动手前告诉它：这条路以前走过，坑在哪里，为什么不能继续补。

### 在 Git conflicts 之前发现逻辑冲突

Git 能发现同一行代码冲突，但发现不了两个 Agent 正在改同一块产品逻辑。

如果别人的 Agent 已经开始调整某个计费规则，虽然还没提 PR，但已经产生了 intent，
其他 Agent 在 preflight 阶段就能看到这件事。冲突不必等到 merge 或 review 才暴露，
一开始就可以换路径、拆边界，或者先对齐。

### 编辑前先读禁区和约束

有些代码看起来很奇怪，注释也不完整，但背后有真实约束。

比如某段 auth middleware 保留了历史兼容逻辑，之前删过一次，结果 rollback。Mainline
会把上次 rollback 的原因、模块约束和相关 intent 放到 Agent 眼前，让它先理解这块代码
为什么不能随便动。

### Review 背后意图

Reviewer 看到的不只是代码 diff，而是实现者的原始目标，思考路径、关键决策。

Review 不再只是在 diff 里猜“它为什么这么写”，而是先看 intent，再验证实现是否真的
符合 intent。代码审查会更早讨论方向、风险和取舍，而不是只在最后挑语法和边界条件。

## Git 原生工作流

Mainline 的存储和协作方式跟 Git 对齐。

| 能力 | Mainline 怎么做 |
|---|---|
| 数据存储于 Git repo | intent event log 存在 Git refs，commit pin 存在 Git notes |
| Git 原生工作流 | fetch、branch、merge、fork 都能带着 intent 走 |
| Agent 全自动读写 | hooks、skill 和 CLI 让 Agent 自己读历史、写判断、封存 intent |
| 不绑某个 vendor | Codex、Claude Code、Cursor、Pi、Copilot 或内部 Agent 都可以按同一份 repo 事实工作 |
| 不改变原有流程 | 你仍然用平常的编辑器、Agent、GitHub / GitLab PR 和 CI |

Mainline 不是把 Git 替掉。它把 Agent 需要继承的工程判断放回 Git。

## Agent 怎么使用 Mainline

Mainline 主要由 Agent 自己操作。开发者在首次安装之后不需要任何额外操作/命令，也不会影响原有的 AI coding 工作流。

它靠三层机制接入现有 AI coding 工作流：

- **Agent hooks**：会话开始时自动同步 repo intent，注入当前状态；
- **Mainline skill**：告诉 Agent 什么时候读历史、什么时候记录判断、什么时候停下来；
- **CLI**：提供可审计的读写命令，比如 `preflight`、`start`、`append`、`seal`。

如果某个 Agent 暂时没有 hook 集成，也可以通过 skill 和 CLI 自然使用 Mainline。
核心原则不变：改代码前先读 Git 里的历史判断，完成工作后再把新的判断写回 Git。

## 完整闭环

一次典型 Agent 工作会长这样：

```bash
mainline preflight --json
mainline start "清理旧导出逻辑" --json
mainline append "确认 legacy CSV 仍被企业客户凌晨对账使用，保留兼容 route" --json
mainline seal --prepare --json > .ml-cache/seal.json
mainline seal --submit --json < .ml-cache/seal.json
```

这几步分别对应：

1. **获取上下文**：Agent 先看当前分支、历史 intent、协作风险和相关约束。
2. **执行任务**：照常读代码、改代码、跑验证、处理 reviewer 反馈。
3. **记录转向**：遇到关键决策、放弃路线、风险变化或验证结果时，用 `append` 留下判断。
4. **密封决策记录**：`seal` 把这次工作整理成可 review、可同步、可被未来 Agent 检索的 intent。
5. **随 Git 流转**：intent 通过 Git refs 和 notes 跟着 fetch、branch、merge 一起进入团队协作。

未来的 Agent 不是从一段旧对话里找线索，而是从 repo 自己携带的历史判断里继承上下文。

## 快速开始

安装 CLI：

```bash
curl -fsSL https://raw.githubusercontent.com/mainline-org/mainline/main/install.sh | bash
mainline doctor --setup
```

也可以用 Go 安装：

```bash
go install github.com/mainline-org/mainline@latest
```

预编译 archive 和 checksums 在
[GitHub Releases](https://github.com/mainline-org/mainline/releases/latest)。

每个 repo 初始化一次：

```bash
cd your-repo
mainline init --actor-name "alice"
```

`mainline init` 会：

- 写入 repo-local Mainline 配置；
- 配置需要的 Git refs 和 notes；
- 安装 Mainline skill；
- 为 Codex、Claude Code、Cursor、Pi 等支持的 Agent 安装 repo-local hooks；
- 把当前 `main` HEAD 记为已有项目的 coverage baseline。

如果一个 repo 是在某个 Agent hook 支持之前初始化的，可以补装 hooks：

```bash
mainline hooks install
# 或只补某个 Agent
mainline hooks install --agent pi
```

全局 skill 需要更新时，使用 `skills` CLI：

```bash
npx --yes skills add mainline-org/mainline --skill mainline --agent codex claude-code cursor pi --global --yes
```

## 人类常用命令

日常大多数命令会由 Agent 跑。人类通常只需要读状态、看 intent、打开 Hub。

```bash
mainline status --actionable
mainline log
mainline show <intent_id>
mainline gaps
mainline hub open
```

打开 Hub 后，可以浏览 intent history、pending work、文件级上下文、coverage gaps、
risks 和协作信号。

静态导出：

```bash
mainline hub export ./mainline-hub
```

公开 Hosted Hub 入口是：https://mainline.sh/hub/

安装变体、恢复规则、hooks 行为、webhooks、配置、静态 Hub 发布、存储布局和开发命令，
放在 [docs/reference.zh.md](./docs/reference.zh.md)。

## 团队协作

Mainline 的多人协作能力来自 Git。

同事、分支、fork、CI、另一个 Agent，只要能拿到同一个 repo，就能拿到同一份 intent
历史。团队成员不需要去问“昨天那个 Agent 为什么这么改”，也不需要翻某个人本地的
聊天记录。

典型协作方式：

- 新人先看 `mainline log` 和 Hub，快速理解最近项目判断；
- Reviewer 先读 intent，再看 diff；
- Agent 在 preflight 阶段发现相邻 intent，避免一开始就写出冲突方案；
- fork contributor 可以发布自己的 actor log，由 upstream maintainer 显式接受；
- merge 后，commit 和 intent 会被 pin 到一起，成为下一轮工作的历史上下文。

外部贡献者、fork PR、actor log import 等细节见
[docs/reference.zh.md](./docs/reference.zh.md)。

## 什么时候用

非微小 Agent 工作前，建议使用 Mainline：

- 架构调整；
- 重构和迁移；
- 删除代码；
- auth、billing、permissions、data model；
- release / CI；
- “这个能不能删？”；
- “以前是不是试过这条路？”；
- 任何可能和另一个 Agent 或队友相邻的工作。

Typo、纯格式化、一行明显语法修复，通常不用。

## 有效果吗

我们做过一轮受控评测：8 个场景，3 个 seed，2 种模式。

| 模式 | forbidden-list violation | 一致性 |
|---|---:|---|
| Intent-first | 0 | 0/8 fixture 失败 |
| Code-first | 9 | 2/8 fixture 稳定失败 |

优势集中在代码看不出来的地方：abandoned approaches、superseded decisions，
以及源码之外的团队约定。

完整方法论和限制条件见 [docs/eval-results.zh.md](./docs/eval-results.zh.md)。

## 继续阅读

- 详细参考：[docs/reference.zh.md](./docs/reference.zh.md)
- Eval 报告：[docs/eval-results.zh.md](./docs/eval-results.zh.md)
- Intent Record Spec：[docs/specs/intent-record-v0.md](./docs/specs/intent-record-v0.md)
- Agent Context Protocol：[docs/specs/agent-context-protocol-v0.md](./docs/specs/agent-context-protocol-v0.md)
- Agent Autonomy Stop Lines：[docs/specs/agent-autonomy-stop-lines-v0.md](./docs/specs/agent-autonomy-stop-lines-v0.md)
- 贡献指南：[CONTRIBUTING.md](./CONTRIBUTING.md)
- 安全：[SECURITY.md](./SECURITY.md)
- Changelog：[CHANGELOG.md](./CHANGELOG.md)

## 开发

```bash
go build -o mainline .
make quick-test
make test
make lint
```

核心子系统有 property-based tests。快速 PR gate 是 `make quick-test`；更完整的 PBT
说明在 [docs/reference.zh.md](./docs/reference.zh.md#开发和测试)。

## 项目状态

Mainline 处于 public alpha。CLI、skill、hooks、Hub、Git refs / notes 存储模型已经
可用，但 schema、agent workflow guidance 和 Hosted Hub 体验仍会继续打磨。

Mainline 适合那些已经把 Agent 用进真实工程流程、但开始担心上下文断掉的团队，期待
您的反馈。

## 授权

Mainline 使用分层授权。本地 CLI、agent skills、hooks、adapters、libraries 和
protocol specs 尽量开放，方便企业、IDE、agent vendor 和自动化平台集成。
Docs 和 examples 鼓励带署名复用。Hosted service 和品牌资产保留独立边界。

详情见 [docs/reference.zh.md](./docs/reference.zh.md#授权细节) 和
[LICENSE](./LICENSE)。

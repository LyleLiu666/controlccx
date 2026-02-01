# ControlCCX 愿景复盘与下一步迭代方向

Last updated: 2026-02-01

> 目的：重新对齐“我们到底要把什么做得更舒服”，并解释为什么现在机制很多但组合起来仍不顺畅；给出下一步高优先级迭代方向与可验收标准。

## TL;DR（先给结论）

目前“不舒服”的核心不是某一个功能缺失，而是**多个正确机制之间缺少一条统一的用户主线（single primary workflow）**：

- 我们把 **provider 的 `session_id`** 当成了 ControlCCX 的“会话/Session 主键”（`tasks.SessionKey()`：`s:<session_id>` / `t:<task_id>`），导致 provider 会话一旦失效或变化（Claude Code 的 `No conversation found…`），用户视角的“同一件事”会被拆成多个 session/runs，恢复路径割裂。
- Run Workspace、Approval/Secretary、Resume、Acceptance Gates 等机制各自都在解决问题，但**缺少“下一步应该点什么”的统一决策层**：用户需要记住每个面板/按钮的语义，并在失败时自己拼流程。
- 速度与安全在系统里有设计（SSE events、sandbox presets、blocked/approval 等），但“默认/例外”的边界不够清晰：用户不容易预测系统会怎么做、为什么这么做。

下一步最值得投的方向（按优先级）：
1) **把“会话”收回到 CCX 自己维护**：引入稳定的 `conversation_id`（或 `thread_id`）作为一级实体；provider `session_id` 变成 run 的一个可变属性；Resume 失败自动降级到 Rehydrate，但仍归属于同一 conversation。
2) **建立统一状态机 + 下一步推荐 CTA**：把 “Resume / Rehydrate / Merge Workspace / Approve / Retry / New Run” 变成系统可计算的“推荐下一步”，UI 只暴露 1 个主按钮 + 解释原因。
3) **把安全做成“可预期、可追溯、可回滚”**：补齐 Secretary Level 2/3 的闭环、把高风险动作的理由/证据显式展示；同时降低日常任务的打断率。
4) **把快做成“感知上的快”**：减少等待点（worker 启动/准备/上下文加载/日志加载），并让 UI 提前给确定性反馈（预计耗时、当前阶段、卡点原因与可操作项）。

## 我们的愿景（重新对齐）

ControlCCX 的使命不是“替代 Claude Code / Codex”，而是把它们变成**可替换的执行引擎**，让用户在一个统一界面里获得：

- **低心智负担**：用户只需要表达“我要做什么”，而不是理解/配置一堆底层机制（session、workspace、approval、intent、sandbox…）。
- **高连续性**：同一件事（一个 conversation）在失败、重启、切换引擎时仍然能继续；不会因为 provider session 失效就“断线换号”。
- **默认安全**：未显式解锁时，系统总在最小权限下运行；遇到不确定/高风险时可解释、可追溯、可升级给用户确认。
- **高确定性**：系统的自动决策（autopilot/secretary）可解释，且用户能用最少步骤完成“恢复 / 合并 / 继续”。

## 为什么现在用起来不顺（问题拆解）

### 1) 心智模型被“实现细节”暴露

用户视角只关心：
- 这是同一件事还是另一件事？（conversation）
- 当前卡在哪？我下一步点什么？（next action）
- 我会不会丢改动/泄露 secrets/误执行危险命令？（safety）

但当前 UI/数据模型把这些问题拆成了多个分散概念：
- task/run（一次执行尝试）
- session（既指 provider session，又被我们用作 UI 分组主键）
- workspace（隔离目录、merge/discard 的状态）
- approvals/blocked（需要通过的门）
- acceptance（迭代收敛机制）

机制都对，但用户需要自己把它们串起来，导致“看起来复杂、用起来慢、失败恢复靠经验”。

### 2) “会话”主键选错导致连续性差（本质根因）

我们当前的 session key：`tasks.SessionKey(taskID, sessionID)`：
- 有 `session_id` → `s:<session_id>`
- 没有 → `t:<task_id>`

这会带来两个体验层面的后果：
- provider session 一旦失效/变更（Claude Code 可发生），UI 视角就变成“另一个 session”，历史上下文被拆散，用户直观感受就是“Resume 失败后又新开了一个 session”。
- 其他以 session key 为锚点的机制（session_meta、session_workspaces、acceptance_states）天然跟着漂移，进一步放大割裂感。

我们最近通过 Rehydrate 提供了“继续干活”的降级通路，但**它仍然会生成新的 provider session**，因此如果 session 的一级标识仍是 `session_id`，割裂感仍然存在。

### 3) 失败恢复不是一等公民（要用户手动拼流程）

典型“卡点组合”：
- Resume → provider session 找不到（`No conversation found…`）
- 同时存在 Run Workspace → 需要先 Merge 才能安全继续
- 同时在安全策略上还可能遇到 blocked/approval

如果系统不告诉用户“先 Merge 再 Rehydrate”，用户要么丢改动，要么来回重试，体验非常差。

### 4) 安全机制的“默认/例外”边界不够清晰

我们已经有：
- sandbox presets / autopilot 意图推断
- blocked（approval required）
- unsafe_automation 全局开关 + 一次性 install unlock

但用户仍会觉得“不够安全”的原因往往不是功能缺失，而是：
- **我不知道系统此刻到底允许什么**（边界不透明）
- **我不知道为什么这次没弹窗/这次又弹了**（决策不可解释）
- **我不知道哪里可回看、出事怎么追责/复盘**（缺可追溯链路）

### 5) “不够快”往往来自等待点 + 不确定性

即便 SSE 已经能把日志实时推送，“感知上的慢”仍可能来自：
- worker 启动/准备阶段缺少明确进度（用户不知道在等什么）
- Resume/Workspace/Approval 等需要用户做决策时，缺少“推荐下一步”导致停顿
- 上下文过长时，provider 端本身慢；系统未提前提示并提供降级（摘要/裁剪/分段）

## 下一步迭代方向（从体验主线出发）

### 方向 A：会话（Conversation）由 CCX 维护（最优先）

**目标**：用户认知里“同一件事”永远不分裂；provider session 只是可变的实现细节。

建议设计：
- 新增稳定主键：`conversation_id`（或 `thread_id`），由 CCX 生成与持久化。
- Task/Run 归属于 conversation；provider `session_id` 记录在 run 上，允许变化（resume/rehydrate/跨引擎切换都不改变 conversation）。
- “Resume / Rehydrate” 统一为一个按钮：**Continue**（系统选择最优路径），并在详情里解释选择依据（例如：`session_id` 存在但 provider 不可用 → 自动 rehydrate）。
- Workspace/Acceptance/Session title 等跟 conversation 绑定，而不是跟 `s:<session_id>` 绑定。

可验收标准（体验/一致性）：
- 同一 conversation 下，无论发生多少次 resume/rehydrate，UI 仍归为一组；用户不会看到“我明明在继续同一件事却变成新 session”。
- 数据层：conversation 相关的 title/workspace/acceptance 不再因为 provider session 变更而丢失或迁移复杂化。

### 方向 B：统一状态机 + 下一步推荐（让机制“组合得顺”）

**目标**：把复杂机制收敛到“系统帮你选下一步”，并把原因讲清楚。

做法：
- 定义 conversation 级别状态（示例）：`running / blocked / needs-merge / resumable / rehydratable / finished`。
- 把“需要用户动作”的点做成显式的 `Action`：`MergeWorkspace`、`Approve`、`Continue`、`Retry`、`NewRun`。
- UI 主线只展示 1 个主按钮（Primary CTA），旁边用 1 行解释“为什么推荐它”；高级选项放二级菜单。

可验收标准（减少选择成本）：
- 新用户不需要理解 “resume vs rehydrate vs merge vs approvals” 的全量语义，也能在失败时被引导到正确操作。
- 关键路径从“观察 → 猜测 → 试错”变成“观察 → 一键下一步 → 可解释”。

### 方向 C：安全：可预期 + 可追溯 + 可回滚

**目标**：默认安全，但不牺牲效率；用户信任系统的自动决策。

重点补齐：
- Secretary Level 2/3 的闭环：自动决策 + 升级面板 + 决策日志（可回看、可检索）。
- 高风险动作的证据展示：例如将 “将要执行的命令/文件变更/diff/网络域名/可能泄露 secrets 的字段” 作为审批依据。
- 基础安全：secrets 处理（脱敏展示、避免写入日志）、危险命令的额外确认、运行环境隔离策略可视化。

可验收标准（信任与控制）：
- 用户能解释“为什么这次系统没打断 / 为什么这次需要我确认”。
- 任何一次自动通过/自动拒绝都能在 system log 中找到理由与上下文。

### 方向 D：速度：把等待点变成“可见、可控、可降级”

**目标**：让用户感知上更快，即使 provider 本身慢，也能减少焦虑与无效等待。

优先级建议（从体验收益最大处开始）：
- UI 显示 run 的阶段与预期：`starting → running → blocked → finishing`（而不是只有 running/failed）。
- 上下文管理：对长会话提供自动摘要/分段/上下文快照（rehydrate 时优先用摘要 + 最近 N 条原文）。
- Worker 启动优化：尽量复用进程/减少冷启动成本（需评估 Claude/Codex CLI 能否常驻或复用认证上下文）。

可验收标准（可量化指标）：
- P50/P90 的 “time-to-first-log” 与 “time-to-first-assistant-token” 可观测，并持续下降。
- 用户主动等待（无下一步提示的停顿）次数下降。

## 统一模型（建议的概念图）

```mermaid
graph TD
  U[用户] --> C[Conversation (CCX 主键)]
  C -->|包含| R1[Run/Task #1]
  C -->|包含| R2[Run/Task #2]
  R1 --> P1[Provider Session ID (可变)]
  R2 --> P2[Provider Session ID (可变)]
  C --> W[Run Workspace (可选, 状态: active/merged/discarded)]
  C --> A[Acceptance & Secretary (策略/状态)]
```

对用户暴露的主线：**Conversation → Continue（系统选 resume/rehydrate）→ 需要时 Merge/Approve → 收敛到可合并结果**。

## 建议的下一步拆解（最小可落地迭代）

### Milestone 1（体验主线）：Conversation 主键 + Continue
- 引入 `conversation_id`，迁移 UI 分组从 `session_id` 迁到 `conversation_id`。
- Continue 按钮：优先 resume；检测到 provider session missing 时自动 rehydrate（必要时提示先 merge）。
- 打通 title/workspace/acceptance 的绑定方式，减少迁移/漂移。

### Milestone 2（更快）：上下文快照 + 进度可见
- 对每个 conversation 维护一个“上下文快照”（摘要 + 关键产物索引 + 最近日志窗口）。
- UI 展示阶段/卡点原因（例如：blocked 的原因、needs-merge 的原因、rehydrate 的来源 run）。

### Milestone 3（更安全）：Secretary 2/3 + 审计/理由
- 把审批决策与证据固化到统一结构（可回看、可导出）。
- 完整跑通“自动处理 80% + 关键节点升级”的体验闭环。

## 需要你确认的关键问题（避免想当然）

为了把“顺畅/快/安全”落到更贴合你的使用习惯，请你选 3–5 个最痛的点回答（越具体越好）：
1) 你最常用的 3 类任务是什么？（例如：改代码/跑测试/查资料/写文档/发布）
2) 你最讨厌的“停顿”发生在哪里？（启动、等待输出、被弹窗打断、合并 workspace、失败恢复）
3) 你对默认安全的容忍度：宁愿多打断还是宁愿少打断？（以及最不能接受的风险是什么）
4) 你认为“同一件事”的边界是什么？（按 repo？按需求？按一天？按你手动命名？）
5) 对 Workspace 的期待：你希望默认隔离还是默认直写？你愿意一键自动 merge 吗？


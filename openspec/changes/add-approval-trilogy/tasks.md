## 0. Scope Guard（很重要）
- [x] 本 change 只做“文档/规格”，不直接实现审批链路代码。
- [x] 明确后续实现会拆分成独立 change（建议至少拆成 2～3 个 change：blocked/MVP、秘书审批、升级给用户）。
  - 已完成：`add-approval-blocked-mvp`（blocked 状态 + UI 引导）、`add-claude-auto-approve-toggle`（Level 1 相关开关）
  - 待完成：Level 2（秘书全审）、Level 3（升级给用户面板 + 交互）

## 1. Spec（把复杂度写清楚）
### 1.1 术语与边界
- [x] 定义“审批请求（approval request）”的最小结构：`id`、`task_id/run_id`、`worker_type`、`workdir`、`action_type`、`risk_level`、`summary`、`raw`（原始工具请求/命令）、`created_at`、`status(pending/approved/denied/escalated/expired)`。
- [x] 定义“三档策略”的行为矩阵（Level 1/2/3 × action_type × risk_level）。
- [x] 明确“New Run 与 Resume 同策略”的约束（避免未来出现两个开关体系打架）。

### 1.2 事件与可追溯性
- [x] 定义必须记录的 trace：每次审批的输入/决策/理由（至少 system log 一条）。
- [x] 定义 SSE 事件（若需要）：`approval.requested` / `approval.updated`（仅写 spec，不实现）。
- [x] 明确“不可隐蔽自动操作”：Level 2/3 也必须可追溯（用户可以事后看见秘书做了什么）。

### 1.3 Secretary 决策策略（先写原则）
- [x] 定义秘书必须保守处理的类型（默认拒绝或升级）：
  - 大范围删除/破坏性改动
  - 可能泄露 secrets 的操作
  - 远端/网络敏感（push/pull/fetch/remote、curl/wget 等）
  - 影响依赖/构建链（安装脚本、执行未知二进制）
- [x] 定义秘书可以自动放行的类型（示例）：
  - 读取文件、搜索、格式化、生成测试、运行只读分析（lint/format/check）
- [x] 定义“无法判断”的条件与升级阈值（例如 diff 太大、未在工作区、风险关键字命中等）。

### 1.4 UX（驾驶室）
- [x] UI 必须能一眼看到：当前处于哪个审批级别（Level 1/2/3），并能快速切换（带文案解释）。
- [x] Level 3 升级给用户时的交互：不弹出复杂对话框；优先用一个紧凑的“Approve / Deny / Details”面板。
- [x] 明确不提供“全部用户审批”模式（写进 UI spec，避免产品回退）。

### 1.5 Validate
- [x] `openspec validate add-approval-trilogy --strict --no-interactive`

## 2. Docs（面向用户的解释要更具体）
- [x] README 增加“什么时候会被升级给用户”的例子（2～4 条）。
- [x] README 明确：Level 1 的风险与适用场景；Level 2/3 的差异。

## 3. Implementation Plan（仅作为后续拆分路线图，不在本 change 实现）
### 3.1 Phase A（MVP：blocked + 手动处理）
- [x] 当 worker 报 “requires approval” 时，任务进入 `blocked`，并写入 system log 原因。
- [x] UI 显示 blocked 的原因与下一步引导（例如：切到 Level 1 或重试）。

### 3.2 Phase B（Level 2：秘书全审）
- [x] 写清楚：如何把 approval request 发给 Observer/Secretary（server-side），由秘书做 approve/deny。
  - 输入：approval request（含 action_type/risk_level/raw）
  - 输出：decision + reason（落 system log）
  - 需要明确“超时/失败”兜底：默认升级给用户或进入 blocked
- [x] 写清楚：任务如何“继续执行”
  - 方案 A（优先）：worker 支持交互式 approve（例如往 stdin 写入 `y/n`），则由系统代为输入继续执行
  - 方案 B（兜底）：worker 不支持继续执行，则通过“重跑策略”替代（记录原始 raw，重新启动 run 并带上允许列表）

### 3.3 Phase C（Level 3：秘书优先 + 升级）
- [x] 写清楚：秘书判断高风险/不确定时，创建 escalation 并通知 UI，让用户做最终确认。
  - UI 交互：紧凑面板 Approve/Deny + Details
  - 用户决策必须落审计（system log），并可回放（看到用户确认了什么）

### 3.4 安全与回滚
- [x] 写清楚：任何自动决策必须可追溯（系统日志/审计记录）。
  - 最低要求：system log + 可查询列表（后续可做 SSE / dashboard）
- [x] 写清楚：任何策略切换必须生效范围明确（对后续 run 生效还是对当前 run 生效）。
  - 建议：策略切换默认只对后续 run 生效；对当前 run 生效需要显式确认（避免“中途换档”造成不可预期）

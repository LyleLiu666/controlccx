## ADDED Requirements

### Requirement: Approval request model
系统 MUST 为每一次“需要审批”的操作创建一个 **approval request** 记录，用于后续决策与可追溯。

最小结构（字段名为示意，最终实现可调整，但语义必须满足）：
- `id`: 唯一 ID
- `task_id` / `run_id`: 关联到哪个 run（本项目当前以 task 表示 run）
- `worker_type`: 触发该请求的 worker 类型（例如 `claude-code` / `codex`）
- `workdir`: 触发该请求时的工作目录
- `action_type`: 操作分类（见下方）
- `risk_level`: 风险分级（low/medium/high）
- `summary`: 面向 UI 的简短说明（可为空）
- `raw`: 原始请求载荷（例如命令行、工具参数、diff 摘要等）
- `created_at`
- `status`: `pending` / `approved` / `denied` / `escalated` / `expired`

建议的 action_type（可扩展）：
- `fs.read` / `fs.write` / `fs.delete`
- `shell.exec`
- `git.remote`（push/pull/fetch/remote 等）
- `net.request`（curl/wget 等）
- `install.deps`（安装依赖/执行安装脚本）
- `unknown`

#### Scenario: Create an approval request
- **GIVEN** worker 在执行过程中触发“需要审批”的操作
- **WHEN** 系统接收到该请求
- **THEN** 系统创建一条 approval request 记录
- **AND** 该记录至少包含 `task_id/run_id`、`action_type`、`risk_level`、`raw` 与 `status=pending`

### Requirement: Three-level approval policy
系统 MUST 支持三档审批策略，用于控制 worker 执行中“需要审批”的操作如何决策。

策略级别：
- Level 1：全自动直通（No Approval）
- Level 2：全交给秘书审批（Secretary Only）
- Level 3：秘书优先，必要时升级给用户（Secretary → User Escalation）

行为矩阵（概念级，便于后续实现保持一致）：

| 风险等级 | Level 1 | Level 2 | Level 3 |
|---|---|---|---|
| low | 自动通过 | 秘书可自动通过 | 秘书可自动通过 |
| medium | 自动通过 | 秘书可通过/拒绝 | 秘书可通过/拒绝 |
| high | 自动通过（高风险） | 秘书默认拒绝或要求更多信息 | 秘书必须升级给用户（默认） |

> 注：Level 2/3 的“默认”是指产品/秘书策略应保守；但最终实现允许在可追溯前提下对具体 action_type 做更细分规则。

#### Scenario: Level 1 auto-pass
- **GIVEN** 用户选择 Level 1（全自动直通）
- **WHEN** worker 产生需要审批的操作
- **THEN** 系统 SHOULD 直接通过，不进入 blocked

#### Scenario: Level 2 secretary-only
- **GIVEN** 用户选择 Level 2（全交给秘书审批）
- **WHEN** worker 产生需要审批的操作
- **THEN** 系统 MUST 将审批请求交由秘书决策
- **AND** 用户不需要每次确认

#### Scenario: Level 3 secretary-first with escalation
- **GIVEN** 用户选择 Level 3（秘书优先，必要时升级给用户）
- **WHEN** worker 产生需要审批的操作
- **THEN** 系统 MUST 先交由秘书决策
- **AND** **IF** 秘书判断无法确定或高风险
- **THEN** 系统 MUST 将该审批升级给用户确认

#### Scenario: No “always user approval” mode
- **WHEN** 用户配置审批策略
- **THEN** 系统 MUST NOT 提供“所有决策都必须用户审批”的策略选项

### Requirement: Traceability and audit log
系统 MUST 对每一次审批请求与决策写入可追溯记录（至少一条 system log），并可在 UI 中被回看。
系统 MUST NOT 在 Level 2/3 下做不可回看的“隐蔽自动操作”。

#### Scenario: Audit every approval decision
- **GIVEN** 一个 approval request 被批准/拒绝/升级/过期
- **WHEN** 系统记录该决策
- **THEN** 系统写入至少一条 system log
- **AND** 日志包含：request id、决策（approve/deny/escalate/expire）与简短理由（如果有）

### Requirement: Approval events (optional)
系统 MUST 定义审批生命周期事件的语义与事件名，以便 Web UI 及时展示（仅定义规格，后续 change 实现）。

#### Scenario: Emit approval events
- **GIVEN** 创建或更新 approval request
- **WHEN** 状态变化发生
- **THEN** 系统可通过 SSE 发送 `approval.requested` / `approval.updated`

### Requirement: Same policy for new and resume
“新建任务”与“Resume/追问” MUST 使用同一审批策略。

#### Scenario: Resume uses the same policy
- **GIVEN** 用户已配置审批策略
- **WHEN** 用户触发 Resume
- **THEN** 新 run MUST 使用相同的审批策略

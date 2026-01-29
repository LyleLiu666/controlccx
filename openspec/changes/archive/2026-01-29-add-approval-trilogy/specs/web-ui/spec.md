## ADDED Requirements

### Requirement: Approval policy setting
Web UI MUST 提供审批策略设置（Level 1/2/3），并持久化。

#### Scenario: Select approval policy
- **WHEN** 用户修改审批策略
- **THEN** 该策略被持久化
- **AND** 后续 New Run / Resume 使用该策略

#### Scenario: Explain policy meaning
- **WHEN** UI 展示审批策略选项
- **THEN** UI MUST 明确解释三档的含义与风险

### Requirement: Always-visible policy indicator
Web UI MUST 让用户“一眼看到”当前处于哪个审批级别（Level 1/2/3），避免误操作。

#### Scenario: Show current policy
- **GIVEN** 用户在主界面
- **WHEN** 查看顶部或设置区域
- **THEN** UI 显示当前审批级别（Level 1/2/3）

### Requirement: Level 3 escalation panel
当处于 Level 3 且秘书升级给用户确认时，UI MUST 用紧凑面板呈现审批请求：
- Approve / Deny 两个主操作
- Details 可展开查看 `summary/raw`

#### Scenario: Approve or deny an escalated request
- **GIVEN** UI 收到 `approval.requested`（status=escalated）
- **WHEN** 用户点击 Approve 或 Deny
- **THEN** UI 发送确认并更新状态（例如通过 `approval.updated` 或刷新状态）

### Requirement: No “always user approval” mode
Web UI MUST NOT 提供“所有决策都必须用户审批”的策略选项。

#### Scenario: Policy options exclude always-user mode
- **WHEN** 用户查看审批策略设置
- **THEN** UI 不展示“全部用户审批”选项

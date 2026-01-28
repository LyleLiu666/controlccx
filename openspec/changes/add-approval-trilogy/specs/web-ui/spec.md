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


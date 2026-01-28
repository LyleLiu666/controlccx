## ADDED Requirements

### Requirement: Secretary approval decision
当审批策略为 Level 2 或 Level 3 时，秘书 MUST 能基于项目上下文判断某个操作是否应被批准或需要升级给用户。

#### Scenario: Identify potentially harmful actions
- **GIVEN** 一个审批请求涉及破坏性改动（例如大范围删除、未提交代码的破坏性操作）
- **WHEN** 秘书评估该请求
- **THEN** 秘书 SHOULD 标记为高风险
- **AND** 在 Level 3 下升级给用户；在 Level 2 下由秘书直接拒绝或要求额外信息

#### Scenario: Require confirmation for remote/network sensitive actions
- **GIVEN** 一个审批请求涉及远端/网络敏感操作（例如 remote/push/pull/fetch）
- **WHEN** 秘书评估该请求
- **THEN** 秘书 SHOULD 默认保守处理（拒绝或升级给用户）


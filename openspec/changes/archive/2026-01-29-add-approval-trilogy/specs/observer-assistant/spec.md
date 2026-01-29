## ADDED Requirements

### Requirement: Secretary approval decision
当审批策略为 Level 2 或 Level 3 时，秘书 MUST 能基于项目上下文判断某个操作是否应被批准或需要升级给用户。

秘书输出 SHOULD 包含：
- `decision`: approve / deny / escalate
- `reason`: 一句话理由（用于 system log 与 UI）
- `risk_level`: low/medium/high（如与输入不同，应说明原因）

#### Scenario: Identify potentially harmful actions
- **GIVEN** 一个审批请求涉及破坏性改动（例如大范围删除、未提交代码的破坏性操作）
- **WHEN** 秘书评估该请求
- **THEN** 秘书 SHOULD 标记为高风险
- **AND** 在 Level 3 下升级给用户；在 Level 2 下由秘书直接拒绝或要求额外信息

#### Scenario: Require confirmation for remote/network sensitive actions
- **GIVEN** 一个审批请求涉及远端/网络敏感操作（例如 remote/push/pull/fetch）
- **WHEN** 秘书评估该请求
- **THEN** 秘书 SHOULD 默认保守处理（拒绝或升级给用户）

#### Scenario: Auto-approve clearly safe actions
- **GIVEN** 一个审批请求属于明显安全的只读操作（例如读取文件、搜索、lint/check、生成测试但不执行危险命令）
- **WHEN** 秘书评估该请求
- **THEN** 秘书 SHOULD 在 Level 2/3 下自动批准

### Requirement: Escalation thresholds (Level 3)
当审批策略为 Level 3 时，秘书 MUST 使用保守阈值决定是否升级给用户。

建议阈值（示例，可随实现迭代）：
- `risk_level=high` → 必须升级
- diff/变更范围过大或 `raw` 无法理解 → 升级
- action 发生在工作区之外（非 allowlist roots）→ 升级

#### Scenario: Escalate on uncertainty
- **GIVEN** 秘书无法从 `summary/raw` 判断该操作真实影响
- **WHEN** 处于 Level 3
- **THEN** 秘书 MUST 升级给用户确认（而不是盲目通过）

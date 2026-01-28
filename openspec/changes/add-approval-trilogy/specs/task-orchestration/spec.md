## ADDED Requirements

### Requirement: Three-level approval policy
系统 MUST 支持三档审批策略，用于控制 worker 执行中“需要审批”的操作如何决策。

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

### Requirement: Same policy for new and resume
“新建任务”与“Resume/追问” MUST 使用同一审批策略。

#### Scenario: Resume uses the same policy
- **GIVEN** 用户已配置审批策略
- **WHEN** 用户触发 Resume
- **THEN** 新 run MUST 使用相同的审批策略


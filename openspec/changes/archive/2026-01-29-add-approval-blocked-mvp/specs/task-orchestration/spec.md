## ADDED Requirements

### Requirement: Block tasks that require approval
当 worker 的执行出现“需要 approval”的信号时，系统 MUST 将该 run 标记为 `blocked`，而不是 `failed`，并记录阻塞原因。

#### Scenario: Claude Code requires approval becomes blocked
- **GIVEN** 一个 `claude-code` run 正在执行
- **WHEN** worker 输出表明 “This command requires approval”
- **THEN** 该任务状态 MUST 变为 `blocked`
- **AND** 系统 MUST 记录一个可追溯的阻塞原因（至少一条 system log）

#### Scenario: Normal failures remain failed
- **GIVEN** 一个 run 结束且 exit_code 非 0
- **WHEN** 输出不包含 “requires approval” 信号
- **THEN** 该任务状态 MUST 为 `failed`


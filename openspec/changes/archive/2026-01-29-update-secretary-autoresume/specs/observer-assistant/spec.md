## ADDED Requirements

### Requirement: Resume intent works without LLM
当 LLM backend 不可用（或 agent 失败回落到启发式逻辑）时，秘书仍 MUST 能在用户表达“继续/恢复/重试”意图时推进任务，而不是仅输出分析文本。

#### Scenario: Resume latest unfinished task without specifying id
- **GIVEN** 系统存在至少一个需要继续的任务（status 为 failed/blocked/interrupted，且该 session 当前无 running/queued run）
- **WHEN** 用户输入“继续/恢复/重试”等意图但未指定 task_id/session_id
- **THEN** 秘书 MUST 自动选择一个最可能相关的任务并执行 resume（创建新的 resume run 并启动）
- **AND** 回复 MUST 明确包含新 run 的 id 与 session_id（或至少一个可定位标识）

#### Scenario: Resume explicit referenced task/session
- **GIVEN** 用户在消息中提供了 task_id/session_id/id 前缀或 prompt 关键词
- **WHEN** 用户表达“继续/恢复/重试”意图
- **THEN** 秘书 MUST 优先对该引用指向的目标执行 resume

#### Scenario: Explain why resume is not possible
- **GIVEN** 目标 session 已删除 / 无 session_id / 或该 session 已有 running/queued run
- **WHEN** 用户请求继续
- **THEN** 秘书 MUST 返回清晰原因，并提示用户下一步可选动作（例如选择别的 session 或等待运行结束）

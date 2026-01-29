## ADDED Requirements

### Requirement: Session rename
系统 SHALL 支持为 session 设置可读名称，并在列表与详情中展示。

实现 SHOULD 以 “session key” 作为稳定标识：
- 当任务存在 `session_id` 时，key = `s:<session_id>`
- 当任务尚无 `session_id` 时，key = `t:<task_id>`

#### Scenario: Rename a session
- **GIVEN** 用户选中一个 session key
- **WHEN** 调用 `POST /api/sessions/{key}/rename` 并提交新名称
- **THEN** 后续读取 task 列表/详情时都能看到该名称

### Requirement: Session delete (soft)
系统 SHALL 支持软删除 session，并默认在列表中隐藏。

#### Scenario: Delete a session
- **GIVEN** 用户选择删除
- **WHEN** 调用 `POST /api/sessions/{key}/delete`
- **THEN** session 从默认会话列表中隐藏

### Requirement: List tasks with deleted sessions hidden by default
系统 SHALL 默认在 `GET /api/tasks` 中隐藏已软删除 session 的任务。

#### Scenario: List tasks without deleted sessions
- **GIVEN** 某 session 已被软删除
- **WHEN** 调用 `GET /api/tasks`
- **THEN** 返回结果中不包含该 session 的任务

#### Scenario: List tasks including deleted sessions
- **GIVEN** 某 session 已被软删除
- **WHEN** 调用 `GET /api/tasks?include_deleted=1`
- **THEN** 返回结果中包含该 session 的任务

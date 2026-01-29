## ADDED Requirements

### Requirement: Tool Selection UI
系统 SHALL 在创建任务时提供 tool 选择入口。

#### Scenario: Select a tool when creating a task
- **GIVEN** 系统存在多个 tool profiles
- **WHEN** 用户打开 New Run
- **THEN** 用户可以选择某个 tool id 来运行

### Requirement: Tool Config UI (args/env) + Persistence
系统 SHALL 提供工具配置界面，支持为每个 tool profile 设置 `command/args/env` 并持久化。

配置界面 SHOULD 展示：
- tool id / driver
- command（可编辑）
- args（可编辑）
- env（key/value 列表，可编辑）

#### Scenario: Select and configure tool
- **GIVEN** 用户创建新任务
- **WHEN** 选择工具并配置参数
- **THEN** 该配置应用于本次运行

#### Scenario: Persist tool config
- **WHEN** 用户保存 tool 配置
- **THEN** 刷新页面或重启服务后配置仍存在

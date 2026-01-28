## ADDED Requirements

### Requirement: Tool registry
系统 SHALL 支持可扩展的 CLI 工具注册与配置。

#### Scenario: Register a new tool
- **GIVEN** 配置了新工具定义
- **WHEN** 系统启动
- **THEN** 用户可在 UI 中选择该工具运行

### Requirement: Tool-specific env config
系统 SHALL 支持每个工具独立的环境变量配置。

#### Scenario: Configure tool env
- **GIVEN** 用户为工具设置环境变量
- **WHEN** 运行该工具
- **THEN** 变量被注入执行环境

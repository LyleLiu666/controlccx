## ADDED Requirements

### Requirement: Tool Registry (Profiles)
系统 SHALL 支持可扩展的 CLI 工具注册与配置（tool profiles）。

一个 tool profile MUST 至少包含：
- `id`：稳定唯一标识（用于任务选择与持久化）
- `driver`：已支持的执行驱动（例如 `claude-code` / `codex` / `exec`）
- `command`：实际执行的可执行文件（允许覆盖默认值）
- `args`：额外参数（追加到 driver 的基础参数之后）
- `env`：该 tool 运行时注入的环境变量

默认 MUST 提供内置 profiles：
- `id=claude-code`（driver=claude-code）
- `id=codex`（driver=codex）

#### Scenario: Default tools are available
- **WHEN** 系统启动且没有用户自定义 tool profiles
- **THEN** 工具列表至少包含 `claude-code` 与 `codex`

#### Scenario: Register a new tool
- **GIVEN** 用户创建一个 `id=claude-cn` 且 `driver=claude-code` 的 tool profile，并配置 `env`（例如 base url）
- **WHEN** 系统启动或刷新工具列表
- **THEN** 用户可在 UI 中选择 `claude-cn` 运行

### Requirement: Tool Persistence
系统 SHALL 持久化 tool profiles，使其在重启后仍可用。

tool profiles MUST 存储在 data dir 下（例如 `~/.controlccx/tools.json`），并以 **原子写入** 的方式更新。

#### Scenario: Persist tools across restarts
- **WHEN** 用户新增/更新 tool profile
- **THEN** 重启服务后仍可看到该 tool profile

### Requirement: Tool Resolution During Run
系统 SHALL 在执行任务时解析 tool profile。

- 任务创建时的 `worker_type` 字段 MUST 被视为 tool id（兼容：内置 tool id 与现有 worker_type 常量一致）。
- 若 tool id 不存在，系统 MUST 拒绝运行并返回错误（不应静默回退到其它工具）。

#### Scenario: Reject run when tool id is missing
- **GIVEN** 用户选择一个不存在的 tool id
- **WHEN** 尝试创建/启动任务
- **THEN** 系统返回错误并且任务不应进入 running 状态

### Requirement: Tool-specific env config
系统 SHALL 支持每个工具独立的环境变量配置。

#### Scenario: Configure tool env
- **GIVEN** 用户为工具设置环境变量
- **WHEN** 运行该工具
- **THEN** 变量被注入执行环境

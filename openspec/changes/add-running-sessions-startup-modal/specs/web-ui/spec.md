## ADDED Requirements

### Requirement: Startup modal for running sessions
当用户刷新/重新进入 Web UI 时，若存在仍在执行中的 session（包含 `queued`/`waiting`/`running`），UI MUST 主动弹出一个轻量弹窗，
列出这些“运行中会话”，以便用户快速接续查看进度；用户也 MUST 能轻松忽略该弹窗并直接新建会话。

#### Scenario: Show running sessions modal on page load
- **GIVEN** 至少存在 1 个 session 内有 run 处于 `queued`/`waiting`/`running`
- **WHEN** 用户加载页面（刷新/重新进入）
- **THEN** UI SHOULD 弹出弹窗列出这些 session（至少展示名称/工作目录与运行状态）
- **AND** 用户点击某个 session，UI MUST 打开该 session 的详情视图（选中该 session 的 latest run）

#### Scenario: Dismiss and start new run
- **GIVEN** “运行中会话”弹窗已弹出
- **WHEN** 用户点击遮罩空白处关闭弹窗
- **THEN** UI MUST 允许用户继续新建会话/新建 run（弹窗不阻塞创建流程）
- **AND** UI MUST NOT 在同一次页面生命周期内重复自动弹出该弹窗

#### Scenario: No running sessions
- **GIVEN** 没有任何 session 处于 `queued`/`waiting`/`running`
- **WHEN** 用户加载页面
- **THEN** UI MUST NOT 展示该弹窗

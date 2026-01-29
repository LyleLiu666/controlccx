## ADDED Requirements

### Requirement: Auto delivery foreman prompt
Web UI SHALL 在某个 run 进入终态时自动触发一次“交付前哨”对话引导，帮助用户确认是否完成与是否需要工业级交付。

#### Scenario: Auto prompt after run finished
- **WHEN** 某个 run 从 `running/queued` 进入终态
- **AND** 用户启用了 Auto Delivery Foreman（默认启用）
- **THEN** UI SHALL 自动发送一条“交付前哨”消息到 Secretary/Observer
- **AND** UI SHALL 展示 Observer 的回复

#### Scenario: Do not duplicate prompts
- **GIVEN** 某个 run 已触发过交付前哨
- **WHEN** 用户刷新页面或重新进入该 session
- **THEN** UI MUST NOT 对同一 run 重复自动触发

#### Scenario: Non-disruptive UX
- **WHEN** UI 自动触发交付前哨
- **THEN** UI SHALL 不强制抢占输入焦点
- **AND** UI SHALL 以轻量提示/抽屉打开方式展示

### Requirement: Toggle auto delivery foreman
Web UI SHALL 提供一个开关以启用/禁用 Auto Delivery Foreman，并持久化设置。

#### Scenario: Disable auto prompt
- **GIVEN** 用户关闭 Auto Delivery Foreman
- **WHEN** 新的 run 进入终态
- **THEN** UI MUST NOT 自动触发交付前哨

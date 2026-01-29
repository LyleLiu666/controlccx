## ADDED Requirements

### Requirement: Preview tabs
系统 SHALL 提供 Markdown / Raw / HTML 三种预览标签，并允许用户切换。

#### Scenario: Switch preview tabs
- **GIVEN** 用户查看结果
- **WHEN** 切换到 Raw 或 HTML
- **THEN** 以对应格式显示内容

### Requirement: HTML sandbox
系统 SHALL 在 HTML 预览中启用安全沙箱，避免脚本越权执行。

#### Scenario: Render HTML safely
- **GIVEN** HTML 内容包含脚本
- **WHEN** 进行 HTML 预览
- **THEN** 脚本不应获得宿主权限

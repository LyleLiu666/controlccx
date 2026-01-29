## ADDED Requirements

### Requirement: Mobile shell baseline
系统 SHALL 在窄屏（例如 ≤ 900px）提供移动端壳层：会话列表默认折叠、主面板占满宽度、抽屉/浮层统一交互。

#### Scenario: Phone width layout
- **GIVEN** 视口宽度 ≤ 900px
- **WHEN** 用户进入控制台
- **THEN** 会话列表默认隐藏并可通过按钮唤起
- **AND** 主面板占满宽度

### Requirement: Touch target sizing
系统 SHALL 规范触控目标尺寸与间距，保证可点击元素在移动端易于操作。

#### Scenario: Tap targets on mobile
- **GIVEN** 视口宽度 ≤ 900px
- **WHEN** 用户操作按钮/开关/标签
- **THEN** 触控目标尺寸不小于 40px

### Requirement: Keyboard safe input
系统 SHALL 在移动端键盘弹出时保持输入区域可见。

#### Scenario: Input focus on mobile
- **GIVEN** 用户在移动端聚焦输入框
- **WHEN** 虚拟键盘弹出
- **THEN** 输入区域仍处于可见区域

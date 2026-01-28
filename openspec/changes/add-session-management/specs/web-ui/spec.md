## ADDED Requirements

### Requirement: Session management controls
系统 SHALL 在会话列表/详情提供重命名与删除入口。

#### Scenario: Access rename/delete controls
- **GIVEN** 用户查看会话列表或详情
- **WHEN** 打开更多操作
- **THEN** 可看到重命名与删除入口

### Requirement: Show/hide deleted sessions
系统 SHALL 支持在会话列表中切换显示/隐藏已软删除的 session（默认隐藏）。

#### Scenario: Toggle deleted sessions visibility
- **GIVEN** 用户在会话列表
- **WHEN** 打开“显示已删除”开关
- **THEN** 会话列表包含已软删除的 session

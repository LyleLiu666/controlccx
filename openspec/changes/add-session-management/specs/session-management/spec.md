## ADDED Requirements

### Requirement: Session rename
系统 SHALL 支持为 session 设置可读名称，并在列表中展示。

#### Scenario: Rename a session
- **GIVEN** 用户选中一个 session
- **WHEN** 修改名称并保存
- **THEN** 列表与详情展示新名称

### Requirement: Session delete (soft)
系统 SHALL 支持软删除 session，并默认在列表中隐藏。

#### Scenario: Delete a session
- **GIVEN** 用户选择删除
- **WHEN** 执行删除
- **THEN** session 从默认列表中隐藏

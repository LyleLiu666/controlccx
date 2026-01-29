## ADDED Requirements

### Requirement: File tree panel
系统 SHALL 在 UI 中提供工作区文件树，支持层级展开与文件选择。

#### Scenario: Browse workspace files
- **GIVEN** 选定工作区
- **WHEN** 用户打开文件树
- **THEN** 显示可展开的目录结构

### Requirement: Basic editor with save
系统 SHALL 提供基础的文件编辑与保存入口。

#### Scenario: Edit and save file
- **GIVEN** 用户打开文本文件
- **WHEN** 编辑并点击保存
- **THEN** 文件内容被写入并提示成功

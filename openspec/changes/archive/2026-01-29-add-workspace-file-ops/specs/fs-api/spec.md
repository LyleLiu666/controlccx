## ADDED Requirements

### Requirement: Workspace file listing
系统 SHALL 提供受限于工作区根目录的文件树/目录列表 API，用于 UI 文件树浏览。

#### Scenario: List directory tree
- **GIVEN** 用户选定工作区根目录
- **WHEN** 请求目录列表（如 `GET /api/fs/entries?path=...&base=...`）
- **THEN** 返回该目录的子目录与文件信息（包含 `kind=dir|file`）
- **AND** 目录在前、文件在后（大小写不敏感排序）
- **AND** 不允许越界到根目录之外（包含 symlink 越界）

### Requirement: Safe path resolution
系统 SHALL 以“base + relative path”方式解析路径，并确保最终解析结果受限于允许的根目录集合（FSRoots）。

#### Scenario: Resolve relative path under base
- **GIVEN** base 为绝对路径且位于允许根目录下
- **WHEN** path 为相对路径
- **THEN** 最终解析为 `abs(base/path)`

#### Scenario: Block path traversal
- **GIVEN** base 位于允许根目录下
- **WHEN** path 尝试使用 `..` 或 symlink 指向根目录之外
- **THEN** 请求被拒绝（HTTP 403 / "path not allowed"）

### Requirement: File read
系统 SHALL 提供文件读取 API，支持文本内容读取与大小限制（用于预览与编辑加载）。

#### Scenario: Read file content
- **GIVEN** 合法的文件路径
- **WHEN** 请求读取（如 `GET /api/fs/read?path=...&base=...`）
- **THEN** 返回文件内容（超出限制则截断并标记）

### Requirement: File write
系统 SHALL 提供文件写入 API，允许创建/覆盖文件（受大小限制与路径限制）。

约束（MVP）：
- 写入仅支持文本（UTF-8 string）
- 单次写入最大 1 MiB（超出返回错误）
- 默认允许覆盖同名文件（覆盖策略由 UI 明确提示）

#### Scenario: Write file
- **GIVEN** 合法的文件路径与内容
- **WHEN** 请求写入（如 `POST /api/fs/write`）
- **THEN** 文件内容被创建或覆盖
- **AND** 超出限制时返回错误

### Requirement: Directory create/delete
系统 SHALL 支持目录创建与文件/目录删除（受路径限制）。

#### Scenario: Create and delete
- **GIVEN** 合法的目录路径
- **WHEN** 创建目录（如 `POST /api/fs/mkdir`）或删除（如 `POST /api/fs/delete`）
- **THEN** 操作成功并返回确认

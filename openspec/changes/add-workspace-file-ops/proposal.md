# Change: Workspace File Tree + Read/Write

## Why
ControlCCX 当前只能选择目录与只读预览，无法在 UI 中组织/编辑工作区文件，用户需要“写入能力”完成闭环。

## What Changes
- 新增工作区文件树（目录层级浏览）
- 提供文件读写 API（读取/保存/新建/删除/创建目录）
- UI 内提供只读预览与基础编辑保存

## Impact
- Affected specs: `fs-api`, `web-ui`
- Affected code: `internal/api`, `internal/fs`, `web/src/App.vue`

# Change: Session Rename & Delete

## Why
会话数量增多后缺少重命名与删除能力，管理成本高。

## What Changes
- 为 session 提供重命名与删除（软删除）能力
- UI 增加会话管理入口

## Impact
- Affected specs: `session-management`, `web-ui`
- Affected code: `internal/tasks`, `internal/api`, `web/src/App.vue`

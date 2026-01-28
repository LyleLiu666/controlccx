# Change: Multi-CLI Tooling + Env Config

## Why
当前仅支持 Claude/Codex 两类工具，无法低成本扩展，也缺少每工具环境变量配置。

## What Changes
- 引入可扩展的 CLI 工具注册/配置
- UI 支持为每个工具配置环境变量与参数

## Impact
- Affected specs: `tooling`, `web-ui`
- Affected code: `internal/worker`, `internal/config`, `web/src/App.vue`

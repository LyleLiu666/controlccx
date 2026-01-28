# Change: Machine Skills Management (Link-based)

## Why
目前机器里存在多套 skills 目录（如 `~/.agents/skills`、`~/.claude/skills`、`~/.codex/skills`），缺少统一的管理入口：
技能是否存在、是否已被某个工具“启用”、是否是坏链路（broken link）等信息难以追踪。

同时，已有实践（如 `npx skills add`）倾向使用软链接（symlink）把“工具可见的 skills 目录”指向统一的 skills 源目录，
以避免重复拷贝与版本漂移。

## What Changes
- 新增“机器 skills 管理”能力：扫描、展示、启用/停用（link/unlink）机器上的 skills
- 默认采用 link（symlink）方式将 skill 从“源目录”暴露到“目标工具目录”（参考 `npx skills add` 的行为）
- 提供最小可用 API + UI，支持 Claude Code / Codex 等多个目标

## Impact
- Affected specs: `skills`, `web-ui`, `fs-api`
- Affected code: `internal/api`, `internal/config`, `internal/*`, `web/src/App.vue`


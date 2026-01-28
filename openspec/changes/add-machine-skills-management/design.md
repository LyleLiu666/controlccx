## Context
目标是把“机器上的 skills”变成一等公民：能被发现、能被启用/停用、能被诊断（坏链路/冲突），并能同时服务多个工具。
当前实践中常见做法是将 `~/.claude/skills/<name>` 软链接到统一的源目录（例如 `~/.agents/skills/<name>`），类似：

```
~/.claude/skills/skill-creator -> ../../.agents/skills/skill-creator
```

## Goals / Non-Goals
- Goals:
  - 统一管理（list/link/unlink）Claude Code / Codex 等工具的 skills
  - 默认使用 symlink，避免重复拷贝与版本漂移
  - 可诊断：识别 broken link、目标已存在但不是 link 等冲突
  - 安全：限制可写路径范围，避免任意文件系统写入
- Non-Goals:
  - 不负责“下载/安装”skills（git clone / marketplace）第一版先不做
  - 不做“每个 task/worker 单独选择 skills”的细粒度隔离（先做机器级）

## Decisions
- 目录模型
  - `source roots`: 默认扫描 `~/.agents/skills`（可配置扩展为多源）
  - `targets`: 至少支持 `claude`（`~/.claude/skills`）与 `codex`（`~/.codex/skills` 或 `$CODEX_HOME/skills`）
- Link 语义
  - link: 在目标目录创建 `skill-name` → `source/skill-name` 的 symlink（尽量用相对链接，行为对齐 `npx skills add`）
  - unlink: 删除目标目录中的该条目（仅当其为 link 或指向受控 source 时允许）
  - broken: 目标条目为 symlink 但指向不存在时标记为 broken（不自动修复）
- 冲突处理
  - 目标已存在且不是 symlink：拒绝覆盖，提示用户手动处理（避免误删用户文件）
- 安全边界
  - API 层进行路径校验：只允许对“目标 skills 根目录”进行有限操作（link/unlink）
  - 仅允许 link 指向“受信 source roots”下的目录
- Windows 支持
  - 优先尝试 symlink；若失败（权限/策略），降级为 copy 或 junction（待定，写入设计并用测试覆盖）

## Risks / Trade-offs
- symlink 的跨平台差异（Windows 权限）会带来实现复杂度 → 通过明确降级策略+测试降低风险
- 扫描多个目录可能带来“同名 skill 冲突” → 需要定义优先级/去重策略

## Migration Plan
1) 先只做扫描 + link/unlink（不自动迁移已有目录内容）
2) 对已有的目标目录条目：仅识别状态，不做破坏性变更

## Open Questions
- Codex 的默认 skills 目录是否统一为 `~/.codex/skills`？是否需要读取 `$CODEX_HOME`？
- 是否需要支持多 source roots（例如 `~/.agents/skills` + `~/.claude/skills` 本地目录）以及优先级？
- Windows 降级策略选择：copy vs junction，是否要做“只读拷贝”防止漂移？


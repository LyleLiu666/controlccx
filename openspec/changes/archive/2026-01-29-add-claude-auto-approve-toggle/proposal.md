## Why
当前 worker 运行方式是“非交互式”（将 prompt 通过 stdin 传入），但 Claude Code 在触发工具调用时会要求 approval。
当不使用 `--dangerously-skip-permissions` 时，会出现：
- 任务直接失败：`Error: This command requires approval`
- 用户误判为卡死/系统不稳定（频繁刷新/重启）

我们需要一个“用户显式选择”的 auto-approve 入口，让驾驶室模式可用，同时不破坏安全边界。

## What Changes
- Web UI 为 `claude-code` 新增一个 **Auto-approve tools** 开关（默认可配置）。
- 该开关必须是“用户显式选择”，并在 UI 上清晰提示其安全含义。
- 服务端根据开关决定是否对 Claude Code 追加 `--dangerously-skip-permissions`。
- 该设置应可持久化（至少在浏览器 localStorage；是否需要服务端持久化由实现决定）。

## Safety / Non-Goals Alignment
- Non-Goal：未经明确确认的自主危险操作。
- 本变更不让系统“自动决定危险操作”，而是提供一个 **用户自愿开启的 auto-approve 模式**。
- 不引入“自动执行测试/自动修复/自动重跑”等更高风险动作（另行提案）。

## Impact
- Frontend：New Run 面板增加一个开关（仅 claude-code 可见），并持久化用户偏好。
- Backend：需要能接收该偏好并在 worker args 中应用；可能涉及 task 元数据字段或配置扩展。
- Logs：`run.start` 会记录实际 args（包含是否启用 skip-permissions），用于可追溯。

## Open Questions (need approval)
1) 默认值：`claude-code` 的 Auto-approve 默认 **开** 还是 **关**？
2) 作用域：仅 “New Run” 入口，还是 “Resume” 也要可控（建议同一开关复用）？
3) `codex` 是否也要提供类似的开关（`--dangerously-bypass-approvals-and-sandbox`）？默认建议先不做。


## Why
目前 `claude-code` 在遇到需要 approval 的工具调用时，会在输出中提示：
`Error: This command requires approval`
但 ControlCCX 的 worker 运行是非交互式的，无法在 CLI 里完成点击批准，因此会导致：
- 任务直接 `failed`（用户误以为系统/模型崩溃）
- 用户频繁刷新页面/重启服务（进一步破坏体验）

我们需要一个最小可用（MVP）的审批链路起点：**先让系统识别“需要审批”并进入 `blocked`**，把“下一步怎么做”说清楚。

## What Changes
Phase A（MVP）只做两件事：
1) **检测 requires approval**：当 worker 输出出现“需要 approval”信号时，将该 run 标记为 `blocked`（而不是 `failed`）。
2) **可见且可操作**：UI 清晰展示 blocked 的原因，并提供下一步引导（例如：切换审批级别到 Level 1 自动直通、或重试/Resume）。

## Non-Goals (Phase A)
- 不实现真正的交互式 approve/deny（仍无法在同一进程内继续执行）
- 不实现秘书自动审批（Level 2）与升级用户（Level 3）
- 不自动执行危险操作；仅提供“引导/按钮”，并要求用户显式选择策略

## Impact
- Backend：worker 需要把“requires approval”识别成 `blocked`，并把原因记录到任务（warning/error 或 system log）。
- Frontend：blocked 状态在 sessions/详情/Live 中更明确；提供“去设置审批策略/重跑”的入口。

## Open Questions
1) blocked 的来源文本：以 stderr / stdout / system log 的哪个字段为准？（建议：从 claude stream-json 的 tool_result/错误文本中识别）
2) blocked 后的“继续方式”：Phase A 只能通过新 run（Resume 或重跑）来继续，是否要在 UI 里直接提供“一键重跑（Level 1）”？（需明确这是用户显式选择）


## Why
当前当 run 被中断/失败时，用户会在 “Needs attention” 里看到提示，但仍需要用户手动理解并点击 Resume/重试。
这与“驾驶室（Cockpit-first）”与“Never feel stuck”的愿景冲突：用户期待系统秘书能够自动介入，让任务继续推进，
只有在“秘书无法自行决策或存在风险”时才升级给用户。

## What Changes
- 增加一套 **Attention Autopilot**（关注项自动驾驶）：
  - 当 session 进入需要关注的状态（优先：`interrupted`），系统 SHOULD 自动尝试恢复（resume/continue）一次。
  - 自动恢复失败时，系统 SHOULD 给出清晰原因并停止自动尝试（避免无限循环/噪音）。
  - 对于需要审批的情况，保持既有审批策略（自动/秘书/升级用户），但 Autopilot 不应“静默失败”。
- 在 Secretary（Overview）中明确展示 Autopilot 状态（开启/关闭、最近一次自动操作与结果），让用户可控且可理解。

## Impact
- Affected specs: `observer-assistant`, `web-ui`
- Affected code (expected): `web/src/App.vue`（前端状态机与自动化队列），必要时补充后端辅助接口


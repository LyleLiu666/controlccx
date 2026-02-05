## Why
刷新页面后，用户容易忽略“后台仍有 run 在跑”的事实，导致：
- 不知道该去哪个 session 看进度
- 误以为系统卡住/丢失运行

该改动直接提升：
- **Resumability and resilience**：刷新后可快速“接续/重新挂载”到 in-flight work
- **Never feel stuck**：明确告知哪些 session 正在运行

## What Changes
- 页面加载完成后（初次刷新/重新进入 UI），**若存在 running/queued/waiting 的 session**，UI 弹出一个轻量 modal：
  - 列出所有“运行中会话”（按活跃度/最新更新时间排序）
  - 点击某个会话可直接打开该会话（选中其 latest run，进入详情查看日志/结果）
  - 点击遮罩空白处可关闭 modal，继续“新建会话/新建 run”
- modal **仅在每次页面加载时触发一次**（不会在后续的手动刷新/状态更新中反复弹出）

## Non-Goals
- 不新增/修改后端 API
- 不改变现有 sessions 列表与首页“运行中”提示的逻辑（只是在 reload 时增加一次提醒/入口）

## Verification
- `pnpm -C web test`
- `pnpm -C web build`
- `pnpm smoke`
- 手工验证：
  - 有至少 1 个 run 处于 queued/waiting/running 时，刷新页面弹窗出现且可打开对应 session
  - 点击遮罩关闭后，可正常新建 run，且本次页面生命周期不再自动弹窗
  - 窄屏（≤900px）弹窗可用、列表不溢出


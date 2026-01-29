## Why
当前 Secretary 在 **LLM backend 不可用**（或 agent 运行失败）时会回落到确定性启发式回答，但这条路径无法调用 `task_resume`，导致用户即使输入“继续/恢复”，也只能得到分析文本，任务无法自动推进——与“驾驶室/秘书”的预期不一致。

目标：在不引入新的危险操作前提下，让 Secretary 在 fallback 情况下也能 **替用户推进任务**，减少等待与手动操作。

## What Changes
- 为 Observer/Secretary 的 fallback（非 agent）路径增加“继续/恢复/重试”意图识别：
  - 优先解析用户消息中的 `task_id/session_id/id 前缀/关键词` 来定位目标
  - 若用户未指定目标，则自动选择“最可能需要继续”的任务（最近更新、非 running/queued、且状态为 failed/blocked/interrupted）
  - 直接触发一次 `task_resume`（创建新的 resume run 并启动），并在回复中说明已做的动作与新 run 信息

## Impact
- 仅后端行为增强（Observer fallback）；不改 API 协议与存储结构。
- 复用现有 `task_resume` 逻辑与安全约束（session 删除/无 session_id/同 session 有 running 等都会拒绝）。

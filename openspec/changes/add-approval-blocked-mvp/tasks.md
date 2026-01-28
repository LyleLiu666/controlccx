## 1. Spec
- [x] 1.1 定义 “requires approval → blocked” 的判定条件（字符串/事件/结构化字段）
- [x] 1.2 定义 blocked 原因的持久化字段（error vs warning vs log）
- [x] 1.3 `openspec validate add-approval-blocked-mvp --strict --no-interactive`

## 2. Backend (TDD)
- [x] 2.1 新增测试：当输出包含 requires approval 时，最终状态为 `blocked` 而非 `failed`
- [x] 2.2 实现检测逻辑（claude-code stdout 解析处优先）
- [x] 2.3 写入可追溯日志：system log 记录 blocked 原因与下一步建议
- [x] 2.4 回归：确保正常失败仍然是 `failed`（不被误判成 blocked）

## 3. Frontend
- [x] 3.1 Sessions 卡片：blocked 展示更显眼，并在 hover/title 里展示原因摘要
- [x] 3.2 Session Detail：blocked 时提供“下一步怎么做”的简短提示（不打扰）
- [x] 3.3 Live：在 milestones 中保留 blocked 关键信息

## 4. Validation
- [x] 4.1 `go test ./...`
- [x] 4.2 `pnpm -C web build` + `pnpm smoke`

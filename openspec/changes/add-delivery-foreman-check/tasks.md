## 1. Spec
- [x] 1.1 补齐 spec deltas（observer-assistant / web-ui）
- [x] 1.2 跑 openspec validate（若 CLI 可用）

## 2. Backend (optional / minimal)
- [x] 2.1 不改变默认安全边界：不自动执行危险操作
- [x] 2.2（可选）补充后端辅助信息：例如 run 终态原因、可用测试命令提示（仅做文本）

## 3. Frontend
- [x] 3.1 增加设置项：Auto Delivery Foreman（默认开）
- [x] 3.2 在 task 进入终态时触发一次自动“秘书引导消息”（并持久化已处理 run，避免重复）
- [x] 3.3 组织上下文（prompt/workdir/status/结果摘要/末尾日志），让 Observer 能判断复杂度与是否完成
- [x] 3.4 UI 不抢焦点：只打开抽屉/提示，不强行 focus 输入

## 4. Validation
- [x] 4.1 `pnpm -C web build` + `pnpm smoke`
- [x] 4.2 `go test ./...`

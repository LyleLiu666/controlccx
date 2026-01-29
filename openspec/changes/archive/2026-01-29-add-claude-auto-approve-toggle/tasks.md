## 1. Spec
- [x] 1.1 增加 spec deltas（web-ui / task-orchestration）
- [x] 1.2 跑 `openspec validate add-claude-auto-approve-toggle --strict --no-interactive`

## 2. Backend
- [x] 2.1 为 task 启动参数增加“auto-approve”字段（实现方式：API 参数 + 存储/传递）
- [x] 2.2 `claude-code` 根据该字段决定是否追加 `--dangerously-skip-permissions`
- [x] 2.3 保持默认安全：未显式开启则不追加
- [x] 2.4 单测覆盖：默认不加 + 开启后加

## 3. Frontend
- [x] 3.1 New Run UI：对 `claude-code` 显示 Auto-approve 开关（并有安全提示）
- [x] 3.2 Resume 入口：复用该开关（同一语义）
- [x] 3.3 localStorage 持久化（key 版本化）

## 4. Validation
- [x] 4.1 `go test ./...`
- [x] 4.2 `pnpm -C web build` + `pnpm smoke`

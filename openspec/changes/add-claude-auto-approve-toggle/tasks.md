## 1. Spec
- [ ] 1.1 增加 spec deltas（web-ui / task-orchestration）
- [ ] 1.2 跑 `openspec validate add-claude-auto-approve-toggle --strict --no-interactive`

## 2. Backend
- [ ] 2.1 为 task 启动参数增加“auto-approve”字段（实现方式：API 参数 + 存储/传递）
- [ ] 2.2 `claude-code` 根据该字段决定是否追加 `--dangerously-skip-permissions`
- [ ] 2.3 保持默认安全：未显式开启则不追加
- [ ] 2.4 单测覆盖：默认不加 + 开启后加

## 3. Frontend
- [ ] 3.1 New Run UI：对 `claude-code` 显示 Auto-approve 开关（并有安全提示）
- [ ] 3.2 Resume 入口：复用该开关（同一语义）
- [ ] 3.3 localStorage 持久化（key 版本化）

## 4. Validation
- [ ] 4.1 `go test ./...`
- [ ] 4.2 `pnpm -C web build` + `pnpm smoke`


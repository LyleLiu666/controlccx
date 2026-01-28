## 1. Spec
- [ ] 1.1 定义工具注册模型（名称/命令/参数/环境变量）
- [ ] 1.2 定义工具配置的 UI 与持久化
- [ ] 1.3 `openspec validate add-tooling-extensibility --strict --no-interactive`

## 2. Backend (TDD)
- [ ] 2.1 配置模型与加载（多工具）
- [ ] 2.2 Worker 构建命令支持扩展工具

## 3. Frontend
- [ ] 3.1 工具配置面板（env/args）
- [ ] 3.2 创建任务时可选工具

## 4. Validation
- [ ] 4.1 `go test ./...`
- [ ] 4.2 `pnpm -C web build` + `pnpm smoke`

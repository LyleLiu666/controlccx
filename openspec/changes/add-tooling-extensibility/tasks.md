## 1. Spec
- [x] 1.1 定义工具注册模型（名称/命令/参数/环境变量）
- [x] 1.2 定义工具配置的 UI 与持久化
- [x] 1.3 `openspec validate add-tooling-extensibility --strict --no-interactive`

## 2. Backend (TDD)
- [x] 2.1 配置模型与加载（多工具）
- [x] 2.2 Worker 构建命令支持扩展工具

## 3. Frontend
- [x] 3.1 工具配置面板（env/args）
- [x] 3.2 创建任务时可选工具

## 4. Validation
- [x] 4.1 `go test ./...`
- [x] 4.2 `pnpm -C web build` + `pnpm smoke`

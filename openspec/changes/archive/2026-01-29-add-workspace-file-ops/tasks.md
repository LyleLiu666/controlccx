## 1. Spec
- [x] 1.1 定义文件树/读写 API 的范围与路径安全约束
- [x] 1.2 定义文件写入的大小限制与覆盖策略
- [x] 1.3 `openspec validate add-workspace-file-ops --strict --no-interactive`

## 2. Backend (TDD)
- [x] 2.1 新增安全路径解析/根目录限制测试
- [x] 2.2 新增文件读取 API（read）
- [x] 2.3 新增文件写入 API（write/create）
- [x] 2.4 新增目录创建与删除 API（mkdir/delete）

## 3. Frontend
- [x] 3.1 文件树面板（可展开层级）
- [x] 3.2 只读预览 + 简易编辑保存
- [x] 3.3 基础操作入口（新建/保存/删除）

## 4. Validation
- [x] 4.1 `go test ./...`
- [x] 4.2 `pnpm -C web build` + `pnpm smoke`

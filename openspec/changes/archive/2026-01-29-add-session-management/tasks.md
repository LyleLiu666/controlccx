## 1. Spec
- [x] 1.1 定义 session 重命名/删除的数据模型与 API
- [x] 1.2 定义软删除的展示/过滤行为
- [x] 1.3 `openspec validate add-session-management --strict --no-interactive`

## 2. Backend (TDD)
- [x] 2.1 数据模型迁移（session_title / deleted_at）
- [x] 2.2 新增重命名/删除 API
- [x] 2.3 查询默认过滤已删除会话

## 3. Frontend
- [x] 3.1 会话列表支持重命名/删除入口
- [x] 3.2 已删除会话可切换显示/隐藏

## 4. Validation
- [x] 4.1 `go test ./...`
- [x] 4.2 `pnpm -C web build` + `pnpm smoke`

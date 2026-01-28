## 1. Spec
- [x] 1.1 明确 skills 的“源目录/目标目录”模型与默认路径
- [x] 1.2 定义 link/unlink/list 的行为（含 broken link 检测与跨平台约束）
- [x] 1.3 `openspec validate add-machine-skills-management --strict --no-interactive`

## 2. Backend (TDD)
- [ ] 2.1 `internal/skills`: 扫描 skills（目录 + symlink）、解析状态（present/linked/broken）
- [ ] 2.2 安全约束：仅允许在允许的 skills 根目录下创建/删除链接
- [ ] 2.3 API: `GET /api/skills`（聚合视图：所有 skills + 各目标状态）
- [ ] 2.4 API: `POST /api/skills/link` / `POST /api/skills/unlink`
- [ ] 2.5 Windows：无法创建 symlink 时的降级策略（copy/junction）与测试

## 3. Frontend
- [ ] 3.1 “Skills” 面板：列出所有 skills（按源目录聚合）与每目标状态
- [ ] 3.2 操作：对某个目标执行 link/unlink；显示失败原因与建议修复

## 4. Validation
- [ ] 4.1 `go test ./...`
- [ ] 4.2 `pnpm -C web build` + `pnpm smoke`

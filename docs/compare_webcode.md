# WebCode vs ControlCCX 差距清单（功能 & UX）

> 目标：找出 WebCode 更“厉害”但 ControlCCX 当前缺失的能力，覆盖功能与体验。
> 方法：静态阅读代码/文档对比（未运行应用）。
> 版本范围：
> - ControlCCX：`/Users/liu_y/code/goProject/controlccx`
> - WebCode：`/Users/liu_y/code/opensource/WebCode`

## 一句话结论
在本轮迭代后，ControlCCX 已补齐 WebCode 的多项关键能力：**移动端壳层、工作区文件读写闭环、Markdown/Raw/HTML 预览、多工具配置与工具级 env、会话管理（重命名/删除）以及更强的 run 观测（Trace/Logs 导出）**。目前主要差距集中在 **上下文管理/模板/快捷操作** 与 **更“工作台化”的文件高级能力（搜索/对比/监控、Monaco 编辑器）**。

---

## 主要缺口（按影响排序）

1) **移动端体验体系**
   - WebCode：专门的移动端适配与触控优化、iOS 视口/键盘兼容、无障碍与减动画支持（`docs/移动端兼容性优化说明.md`）。
   - ControlCCX：已加入 Mobile Shell（抽屉/顶部导航/触控目标等基础适配，`web/src/App.vue`），但仍缺少更系统性的移动交互（键盘遮挡、无障碍、动效降级）。

2) **工作区文件管理（文件树/上传下载/搜索/对比/监控）**
   - WebCode：文件树、上传/下载、文件对比、文件搜索、文件监控面板、创建/删除文件夹等（`WebCodeCli/Pages/CodeAssistant.razor`，`WebCodeCli/Components/FileSearchPanel.razor`，`WebCodeCli/Components/DiffViewerPanel.razor`，`WebCodeCli/Components/FileMonitorPanel.razor`）。
   - ControlCCX：已具备文件树浏览、预览/编辑、mkdir/delete/write 等读写闭环（`web/src/App.vue` + `internal/api/api.go`）。剩余差距：文件搜索、diff viewer、文件监控、上传/下载（更偏“工作台”能力）。

3) **代码/内容预览能力（Monaco + HTML 预览 + 多 Tab）**
   - WebCode：Monaco Editor 代码高亮、Markdown/原始/HTML 预览多 Tab（`README.md`，`WebCodeCli/Pages/CodeAssistant.razor`）。
   - ControlCCX：已支持 Markdown/Raw/HTML 多 Tab 预览，并在文件编辑中提供轻量 textarea（`web/src/App.vue`）。剩余差距：Monaco 级编辑体验（大文件、语言服务、Diff 体验）。

4) **多 CLI 工具与扩展体系**
   - WebCode：多 CLI 工具适配（Claude/Codex/Copilot/Qwen/Gemini 等），配置化扩展（`README.md`，`WebCodeCli.Domain/Domain/Service/Adapters/`）。
   - ControlCCX：已支持 Tools 配置与多工具适配（driver/command/args/env，可在 UI 管理，`internal/tooling/*` + `web/src/App.vue`）。

5) **工具级配置与环境变量管理**
   - WebCode：在 UI 中为每个 CLI 工具配置环境变量并持久化（`docs/环境变量配置功能说明.md`，`WebCodeCli/Components/EnvironmentVariableConfigModal.razor`）。
   - ControlCCX：已支持 tool-level env 管理与持久化（Tools 面板），并与 Worker/Run 绑定（`internal/tooling/*` + `web/src/App.vue`）。

6) **上下文管理/模板/快捷操作的生产力功能**
   - WebCode：上下文面板、上下文压缩、模板库、快捷操作（`WebCodeCli/README_上下文管理.md`，`WebCodeCli/Components/ContextPreviewPanel.razor`，`WebCodeCli/Components/TemplateLibraryModal.razor`，`WebCodeCli/Components/QuickActionsPanel.razor`）。
   - ControlCCX：目前仍缺少这类“生产力层”模块；这是下一阶段最值得补齐的差距（偏驾驶室：引导、快捷键、可执行下一步、模板化复用）。

7) **会话管理 UX（重命名/删除/用户身份）**
   - WebCode：会话重命名、删除、当前用户/退出（`WebCodeCli/Pages/CodeAssistant.razor`）。
   - ControlCCX：已支持会话重命名/删除（软删除）与筛选（`web/src/App.vue`）。

---

## 详细对照表

| 领域 | WebCode 能力 | ControlCCX 现状 | 缺口影响 |
|---|---|---|---|
| 移动端适配 | 触控目标、iOS 视口、键盘与刘海屏、滚动体验、无障碍（`docs/移动端兼容性优化说明.md`） | 基础响应式 + 抽屉（`web/src/App.vue`） | 手机/平板体验明显差；“随时随地编程”场景缺失 |
| 工作区文件树 | 文件树视图、层级展开、文件选择（`WebCodeCli/Pages/CodeAssistant.razor`） | 无文件树，仅目录选择 + 只读预览 | 无法在 UI 中组织/浏览工作区结构 |
| 文件操作 | 上传/下载、创建/删除文件夹、批量下载（`WebCodeCli/Pages/CodeAssistant.razor`） | 已支持 mkdir/delete/write（`internal/api/api.go` + `web/src/App.vue`），仍缺上传/下载（文件级） | “闭环”已成立，但效率/可移植性仍可提升 |
| 文件搜索/对比/监控 | 搜索面板、Diff、文件监控（`WebCodeCli/Components/FileSearchPanel.razor` 等） | 仍缺对应模块 | 无法在 UI 内做高阶文件分析 |
| 预览能力 | Monaco + Markdown + 原始 + HTML 预览（`README.md`，`WebCodeCli/Pages/CodeAssistant.razor`） | 已支持 Markdown/Raw/HTML 多 Tab；编辑为轻量 textarea | Monaco 级编辑体验仍有差距 |
| 工具扩展 | 多 CLI 工具适配与配置（`README.md`） | 已支持 tools 配置（driver/command/args/env） | 继续扩展更多 driver/适配器需要额外工作 |
| 工具级环境变量 | UI 配置并持久化（`docs/环境变量配置功能说明.md`） | 已支持 tool-level env | 差距收敛 |
| 生产力功能 | 上下文管理、模板库、快捷操作（`WebCodeCli/Components/*`） | 仍缺 | 成熟度与效率差距明显（下一阶段优先） |
| 会话管理 | 重命名、删除、用户管理入口（`WebCodeCli/Pages/CodeAssistant.razor`） | 已支持会话重命名/删除（软删除） | 多用户/账号体系仍未覆盖（若需要） |

---

## 附：ControlCCX 现有但 WebCode 文档未突出
（用于避免误判；不代表 WebCode 没有，只是文档/代码未看到）

- **多任务并行与运行观察**：任务队列、run 列表、Live Feed、Secretary 观测与总结（`web/src/App.vue`）。
- **Claude/Codex 运行日志分流**：stdout/stderr/assistant/system 分类过滤（`web/src/App.vue`）。

---

## 备注与下一步建议
如果需要，我可以基于以上缺口输出：
- 最小可行补齐清单（MVP）
- UX 低成本提升方案（Mobile + 文件树 + HTML 预览优先）
- 详细需求拆分（后端 API / 前端组件 / 数据模型）

---

## Roadmap（OpenSpec / 开发顺序与依赖）

> 迭代循环（每个 change 都遵循）：
> 做文档 → 基于地基/上层关系安排顺序 → 执行开发 → 测试 → 修复 → 回写文档（并勾掉 tasks）

### Phase 1：移动端壳层（地基）
- Change: `openspec/changes/add-mobile-shell/`
- 覆盖缺口：**移动端体验体系**
- 目标：先把“驾驶室”在手机上变得稳且不乱（抽屉、触控目标、键盘遮挡）

### Phase 2：工作区文件体系（闭环：允许写入）
- Change: `openspec/changes/add-workspace-file-ops/`
- 覆盖缺口：**文件树/上传下载/搜索/对比/监控**（先从“文件树 + 读写/新建/删除/创建目录”做起）
- 关键约束：必须有**路径安全与根目录限制**，防止越权读写

### Phase 3：预览能力升级（更接近工作台）
- Change: `openspec/changes/add-preview-tabs/`
- 覆盖缺口：**Markdown/Raw/HTML 多 Tab 预览**
- 关键约束：HTML 预览需要 sandbox（避免脚本越权）

### Phase 4：会话管理（后排，但必须做出来）
- Change: `openspec/changes/add-session-management/`
- 覆盖缺口：**会话重命名/删除**
- 说明：放后排，不阻塞前 3 个 Phase 的“可用性 + 文件闭环”

### Phase 5：工具扩展体系（后排）
- Change: `openspec/changes/add-tooling-extensibility/`
- 覆盖缺口：**多 CLI 工具适配 + 工具级环境变量管理**
- 说明：纳入计划，但排在后面（先把核心体验与闭环做稳）

### Phase 6：生产力层（驾驶室优先）
- 建议新增 Change（待定）：上下文面板 / 模板库 / Quick Actions（更偏“驾驶室”，用于减少用户无聊与手动操作）

### Phase 7：文件高级能力（工作台化，可选）
- 建议新增 Change（待定）：文件搜索 / diff viewer / 文件监控（更偏“工作台”，看需求取舍）

### Phase 8：编辑体验升级（工作台化，可选）
- 建议新增 Change（待定）：Monaco Editor（或更轻量替代），与 Diff 结合

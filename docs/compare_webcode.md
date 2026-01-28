# WebCode vs ControlCCX 差距清单（功能 & UX）

> 目标：找出 WebCode 更“厉害”但 ControlCCX 当前缺失的能力，覆盖功能与体验。
> 方法：静态阅读代码/文档对比（未运行应用）。
> 版本范围：
> - ControlCCX：`/Users/liu_y/code/goProject/controlccx`
> - WebCode：`/Users/liu_y/code/opensource/WebCode`

## 一句话结论
WebCode 在 **移动端体验、工作区文件管理/预览、可扩展 CLI 工具体系、上下文与快捷操作、会话管理 UX** 等方面明显更完整；ControlCCX 更像“控制台+观测器”，但在“编程工作台”与“移动场景”上功能/体验断层。

---

## 主要缺口（按影响排序）

1) **移动端体验体系**
   - WebCode：专门的移动端适配与触控优化、iOS 视口/键盘兼容、无障碍与减动画支持（`docs/移动端兼容性优化说明.md`）。
   - ControlCCX：仅基于宽度判断 `isPhone` 与少量 media query（`web/src/App.vue`），未见系统性的移动交互优化。

2) **工作区文件管理（文件树/上传下载/搜索/对比/监控）**
   - WebCode：文件树、上传/下载、文件对比、文件搜索、文件监控面板、创建/删除文件夹等（`WebCodeCli/Pages/CodeAssistant.razor`，`WebCodeCli/Components/FileSearchPanel.razor`，`WebCodeCli/Components/DiffViewerPanel.razor`，`WebCodeCli/Components/FileMonitorPanel.razor`）。
   - ControlCCX：仅提供目录选择与只读文件预览（`web/src/App.vue`），后端只有 `fs/roots|list|read` 且无写入接口（`internal/api/api.go`）。

3) **代码/内容预览能力（Monaco + HTML 预览 + 多 Tab）**
   - WebCode：Monaco Editor 代码高亮、Markdown/原始/HTML 预览多 Tab（`README.md`，`WebCodeCli/Pages/CodeAssistant.razor`）。
   - ControlCCX：Markdown 渲染 + Highlight.js 的只读预览，无编辑器与 HTML 预览（`web/src/App.vue`）。

4) **多 CLI 工具与扩展体系**
   - WebCode：多 CLI 工具适配（Claude/Codex/Copilot/Qwen/Gemini 等），配置化扩展（`README.md`，`WebCodeCli.Domain/Domain/Service/Adapters/`）。
   - ControlCCX：WorkerType 仅 `claude-code` / `codex`（`web/src/types.ts`）。

5) **工具级配置与环境变量管理**
   - WebCode：在 UI 中为每个 CLI 工具配置环境变量并持久化（`docs/环境变量配置功能说明.md`，`WebCodeCli/Components/EnvironmentVariableConfigModal.razor`）。
   - ControlCCX：仅提供 Claude/Codex 的全局 API Key/Model 设置（`web/src/App.vue`），未见多工具级配置能力。

6) **上下文管理/模板/快捷操作的生产力功能**
   - WebCode：上下文面板、上下文压缩、模板库、快捷操作（`WebCodeCli/README_上下文管理.md`，`WebCodeCli/Components/ContextPreviewPanel.razor`，`WebCodeCli/Components/TemplateLibraryModal.razor`，`WebCodeCli/Components/QuickActionsPanel.razor`）。
   - ControlCCX：无对应 UI/组件与逻辑。

7) **会话管理 UX（重命名/删除/用户身份）**
   - WebCode：会话重命名、删除、当前用户/退出（`WebCodeCli/Pages/CodeAssistant.razor`）。
   - ControlCCX：会话列表支持筛选/固定，但无重命名/删除入口（`web/src/App.vue`）。

---

## 详细对照表

| 领域 | WebCode 能力 | ControlCCX 现状 | 缺口影响 |
|---|---|---|---|
| 移动端适配 | 触控目标、iOS 视口、键盘与刘海屏、滚动体验、无障碍（`docs/移动端兼容性优化说明.md`） | 基础响应式 + 抽屉（`web/src/App.vue`） | 手机/平板体验明显差；“随时随地编程”场景缺失 |
| 工作区文件树 | 文件树视图、层级展开、文件选择（`WebCodeCli/Pages/CodeAssistant.razor`） | 无文件树，仅目录选择 + 只读预览 | 无法在 UI 中组织/浏览工作区结构 |
| 文件操作 | 上传/下载、创建/删除文件夹、批量下载（`WebCodeCli/Pages/CodeAssistant.razor`） | 无上传/下载/写入 API（`internal/api/api.go`） | 只能读文件，无法完成“工作区闭环” |
| 文件搜索/对比/监控 | 搜索面板、Diff、文件监控（`WebCodeCli/Components/FileSearchPanel.razor` 等） | 无对应模块 | 无法在 UI 内做高阶文件分析 |
| 预览能力 | Monaco + Markdown + 原始 + HTML 预览（`README.md`，`WebCodeCli/Pages/CodeAssistant.razor`） | Markdown + Highlight.js，缺 HTML/编辑器 | 生成前端页面无法直接预览 |
| 工具扩展 | 多 CLI 工具适配与配置（`README.md`） | 仅 Claude/Codex 两类（`web/src/types.ts`） | 新工具接入成本高 |
| 工具级环境变量 | UI 配置并持久化（`docs/环境变量配置功能说明.md`） | 只有全局 Key/Model（`web/src/App.vue`） | 难以多工具/多环境切换 |
| 生产力功能 | 上下文管理、模板库、快捷操作（`WebCodeCli/Components/*`） | 无 | 成熟度与效率差距明显 |
| 会话管理 | 重命名、删除、用户管理入口（`WebCodeCli/Pages/CodeAssistant.razor`） | 会话筛选/固定，但无重命名/删除 | 运营与多用户场景不足 |

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

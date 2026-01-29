## Why
ControlCCX 的 worker 运行是“非交互式”形态（stdin + logs），但真实的工程任务经常涉及：
- 文件删除/大改动
- 未提交代码的破坏性操作
- 网络/远端（pull/push、remote 等）
- 执行脚本/安装依赖

如果没有明确的审批机制，要么：
- 为了能跑通而直接全放行（安全风险），要么
- 因为需要交互 approval 而频繁失败（体验差）

因此需要一个“驾驶室”式、与秘书（Observer/Secretary）协作的分级审批策略，既可用又可控。

## What Changes
在文档/规格中明确 **三档审批流程（“三集”）**，用于指导后续实现：

### Level 1：全自动直通（No Approval）
- 所有工具调用/敏感操作默认直接通过
- 适用于用户明确选择“我愿意承担风险，追求效率”
- 典型对应：Claude Code `--dangerously-skip-permissions` / Codex 类似 bypass（如未来支持）

### Level 2：全交给秘书审批（Secretary Only）
- 所有需要审批的操作先交由秘书判断并自动决定
- 用户不参与每一次审批
- 适用于“信任秘书 + 追求不中断”

### Level 3：秘书优先，必要时升级给用户（Secretary → User Escalation）
- 秘书先审批；当秘书无法确定或判断为高风险时，升级给用户确认
- 典型高风险：大范围删除、破坏性改动、对远端/网络敏感操作、可能泄露 secrets 的操作等

### 明确不提供的模式
- **不提供**“所有决策都必须用户审批”的模式（会严重破坏驾驶室体验）

### 统一语义
- “新建任务”与“追问/Resume”使用同一审批策略，不做额外区分

## Safety Notes
- Level 1 是用户显式选择的“风险换效率”，需要 UI 文案清晰说明。
- Level 2/3 虽然减少用户打断，但仍必须满足 Non-Goals：系统不得在用户不知情的情况下做隐蔽的高风险动作；应可追溯并可回放。

## Impact (docs only)
本变更只把“三集审批流程”加入文档/规格，用于后续开发：
- Web UI 的设置与展示
- Worker 执行与 blocked/approval 机制
- Observer/Secretary 的判断与升级策略


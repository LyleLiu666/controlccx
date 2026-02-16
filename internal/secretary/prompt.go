package secretary

import (
	"fmt"
	"strings"

	sectools "controlccx/internal/secretary/tools"
)

func buildSystemPrompt() string {
	var b strings.Builder
	b.WriteString("你是 ControlCCX 的系统级秘书（一个具备工具调用能力的 Agent）。\n\n")
	b.WriteString("你的目标：用自然语言回答用户关于系统/任务的查询，并在用户明确要求时执行任务相关操作（新建、恢复、审批、诊断、任务契约）。\n\n")
	b.WriteString("交互与决策原则：\n")
	b.WriteString("1) 你不能编造任何任务/系统数据。需要数据时必须调用工具获取。\n")
	b.WriteString("2) 信息不足时，一次只问一个关键问题；提问时采用多选引导 + 一句猜测。\n")
	b.WriteString("3) 当用户只说“迭代一下/优化一下”等模糊指令时，不要直接写操作，先澄清方向与验收标准。\n")
	b.WriteString("4) worker_type 语义：claude-code=Claude Code 代理执行；codex=Codex 代理执行；exec=在本机 workdir 直接执行你提供的 shell（bash）命令（原样交给 Unix 的 sh -lc / Windows 的 cmd.exe /c；不会做自然语言转译）。\n")
	b.WriteString("5) worker 选择建议：简单且追求速度 -> claude-code；严肃/生产级迭代 -> codex；不确定则先问再提。\n")
	b.WriteString("6) exec 仅可在用户明确要求执行具体 shell 命令时使用；prompt 必须是可直接执行的命令字符串；自动推荐不使用 exec。\n")
	b.WriteString("7) 你应该替用户做决策，降低用户心智负担：写操作若缺参，优先调用工具推断并选择安全默认（例如 workdir：优先 tasks_list 找最近相关项目/任务的 workdir，其次 fs_pwd）。只有在同时存在多个候选、或会导致高风险/不可逆结果时才向用户确认。\n")
	b.WriteString("8) 写操作前先给执行摘要（目标/worker/验收/关键假设）。一般在用户已明确要执行时可直接提交；只有高风险动作或高度歧义时才先征求确认。\n")
	b.WriteString("9) 高风险动作必须遵守工具参数约束（例如 enter-unsafe 需要 confirm=true）。\n\n")
	b.WriteString("10) “取消/停止”只能使用 task_cancel_submit 取消当前任务；禁止用 task_continue_submit 发送 /cancel 冒充取消。\n")
	b.WriteString("11) updated_at 只是心跳，不等于有进度；判断“卡住”必须结合任务状态与日志。若 task_logs_tail 显示截断/信息不足，必须使用 task_log_get 精查。\n\n")
	b.WriteString("12) 当用户询问本机文件/目录时，优先使用 fs_pwd、fs_roots、fs_entries、fs_read_text 做只读探查；不要声称你无法枚举文件系统。若要列目录但 path 未提供，先调用 fs_pwd。\n\n")
	b.WriteString("输出约束：\n")
	b.WriteString("1) 当你需要调用工具时，你必须只输出一个 <tool_data>...</tool_data> 块，除此之外不要输出任何解释文字。\n")
	b.WriteString("2) 当你要给用户最终答复时，只输出中文纯文本（不要输出 <tool_data>、不要输出 XML 标签、不要输出 Markdown）。\n\n")
	b.WriteString("工具调用格式（示例）：\n")
	b.WriteString("<tool_data>\n")
	b.WriteString("  <call>\n")
	b.WriteString("    <tool_name>tasks_count</tool_name>\n")
	b.WriteString("    <status>succeeded</status>\n")
	b.WriteString("  </call>\n")
	b.WriteString("</tool_data>\n\n")
	b.WriteString("工具说明：\n")
	b.WriteString("安全参数组 [runopts]：" + strings.Join(sectools.RunOptsParams, "、") + "\n")
	for _, d := range sectools.Descriptors() {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(d.DescriptionZH)
		if desc == "" {
			desc = "（无描述）"
		}

		returns := strings.TrimSpace(d.ReturnsZH)
		returnsSuffix := ""
		if returns != "" {
			returnsSuffix = " 返回：" + returns
			if !strings.HasSuffix(returnsSuffix, "。") {
				returnsSuffix += "。"
			}
		}

		params := renderToolParams(d.Params)
		suffix := renderToolParamSuffix(d.Required, d.AnyOfRequired)
		if len(params) > 0 {
			b.WriteString(fmt.Sprintf("- %s(%s)：%s%s%s\n", name, strings.Join(params, ", "), desc, returnsSuffix, suffix))
			continue
		}
		b.WriteString(fmt.Sprintf("- %s：%s%s%s\n", name, desc, returnsSuffix, suffix))
	}
	b.WriteString("\n如果用户问“总共有多少任务/完成多少/失败多少”，优先调用 tasks_count，再用中文给出明确数字。")
	b.WriteString("\n如果用户问“有哪些 skills 可调动/可用”，优先调用 skills_list（如需给出可用清单，建议 target=codex + only_enabled=true），再用中文列出技能名与状态摘要。")
	return b.String()
}

func renderToolParams(params []string) []string {
	if len(params) == 0 {
		return nil
	}

	runOptsSet := make(map[string]struct{}, len(sectools.RunOptsParams))
	for _, key := range sectools.RunOptsParams {
		runOptsSet[key] = struct{}{}
	}
	paramSet := make(map[string]struct{}, len(params))
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		paramSet[p] = struct{}{}
	}

	hasAllRunOpts := true
	for key := range runOptsSet {
		if _, ok := paramSet[key]; !ok {
			hasAllRunOpts = false
			break
		}
	}

	out := make([]string, 0, len(params))
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if hasAllRunOpts {
			if _, ok := runOptsSet[p]; ok {
				continue
			}
		}
		out = append(out, p)
	}
	if hasAllRunOpts {
		out = append(out, "[runopts]")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderToolParamSuffix(required []string, anyOfRequired [][]string) string {
	var b strings.Builder
	required = trimToolParams(required)
	anyOfRequired = trimToolParamGroups(anyOfRequired)

	if len(required) > 0 {
		b.WriteString(" 必填：")
		b.WriteString(strings.Join(required, "、"))
		b.WriteString("。")
	}
	for _, group := range anyOfRequired {
		if len(group) == 0 {
			continue
		}
		b.WriteString(" 必填（任选一组）：")
		b.WriteString(strings.Join(group, " | "))
		b.WriteString("。")
	}
	return b.String()
}

func trimToolParams(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimToolParamGroups(in [][]string) [][]string {
	out := make([][]string, 0, len(in))
	for _, group := range in {
		trimmed := trimToolParams(group)
		if len(trimmed) == 0 {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

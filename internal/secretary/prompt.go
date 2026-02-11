package secretary

import (
	"fmt"
	"strings"

	sectools "controlccx/internal/secretary/tools"
)

func buildSystemPrompt() string {
	var b strings.Builder
	b.WriteString("你是 ControlCCX 的系统级秘书（一个具备工具调用能力的 Agent）。\n\n")
	b.WriteString("你的目标：用自然语言回答用户关于系统/任务的查询，并在用户明确要求时执行任务相关操作（恢复、审批、诊断）。\n\n")
	b.WriteString("重要规则：\n")
	b.WriteString("1) 你不能编造任何任务/系统数据。需要数据时必须调用工具获取。\n")
	b.WriteString("2) 当你需要调用工具时，你必须只输出一个 <tool_data>...</tool_data> 块，除此之外不要输出任何解释文字。\n")
	b.WriteString("3) 当你要给用户最终答复时，只输出中文纯文本（不要输出 <tool_data>、不要输出 XML 标签、不要输出 Markdown）。\n")
	b.WriteString("4) 高风险动作必须遵守工具参数约束（例如 enter-unsafe 需要 confirm=true）。\n\n")
	b.WriteString("工具调用格式（示例）：\n")
	b.WriteString("<tool_data>\n")
	b.WriteString("  <call>\n")
	b.WriteString("    <tool_name>tasks_count</tool_name>\n")
	b.WriteString("    <status>succeeded</status>\n")
	b.WriteString("  </call>\n")
	b.WriteString("</tool_data>\n\n")
	b.WriteString("工具说明：\n")
	for _, d := range sectools.Descriptors() {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(d.DescriptionZH)
		if desc == "" {
			desc = "（无描述）"
		}
		b.WriteString(fmt.Sprintf("- %s：%s\n", name, desc))
	}
	b.WriteString("\n如果用户问“总共有多少任务/完成多少/失败多少”，优先调用 tasks_count，再用中文给出明确数字。")
	return b.String()
}

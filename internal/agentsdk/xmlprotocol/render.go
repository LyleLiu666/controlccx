package xmlprotocol

import (
	"sort"
	"strings"
)

var canonicalFieldAliases = map[string]string{
	"file_path":          "filePath",
	"filepath":           "filePath",
	"replace_all":        "replaceAll",
	"jobId":              "job_id",
	"waitMs":             "wait_ms",
	"waitSeconds":        "wait_seconds",
	"maxRuntimeMs":       "max_runtime_ms",
	"maxRuntimeSeconds":  "max_runtime_seconds",
	"maxLogBytes":        "max_log_bytes",
	"stdoutOffset":       "stdout_offset",
	"stderrOffset":       "stderr_offset",
	"maxDeltaBytes":      "max_delta_bytes",
	"maxResults":         "max_results",
	"fixedStrings":       "fixed_strings",
	"contextSummary":     "context_summary",
	"toolIds":            "tool_ids",
	"maxSteps":           "max_steps",
	"kSkills":            "k_skills",
	"skillIds":           "skill_ids",
	"taskId":             "task_id",
	"offsetLines":        "offset_lines",
	"limitLines":         "limit_lines",
	"maxBytes":           "max_bytes",
	"workerType":         "worker_type",
	"conversationId":     "conversation_id",
	"workDir":            "workdir",
	"work_dir":           "workdir",
	"cwd":                "workdir",
	"dir":                "workdir",
	"worker":             "worker_type",
	"waitMsBeforeCancel": "wait_ms_before_cancel",
}

// RenderCanonicalToolData renders a deterministic <tool_data> XML block for calls.
// It is intended for feeding repaired protocol back into model context.
func RenderCanonicalToolData(calls []Call) string {
	if len(calls) == 0 {
		return "<tool_data></tool_data>"
	}

	var b strings.Builder
	b.WriteString("<tool_data>\n")
	for _, call := range calls {
		b.WriteString("  <call>\n")
		b.WriteString("    <tool_name>")
		b.WriteString(escapeXMLText(call.ToolName))
		b.WriteString("</tool_name>\n")

		fields := canonicalizeFieldsForRendering(call.Fields)
		if len(fields) > 0 {
			keys := make([]string, 0, len(fields))
			for k := range fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("    <")
				b.WriteString(k)
				b.WriteString(">")
				b.WriteString(escapeXMLText(fields[k]))
				b.WriteString("</")
				b.WriteString(k)
				b.WriteString(">\n")
			}
		}

		b.WriteString("  </call>\n")
	}
	b.WriteString("</tool_data>")
	return b.String()
}

func canonicalizeFieldsForRendering(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}

	out := make(map[string]string, len(fields))
	for k, v := range fields {
		out[k] = v
	}

	for alias, canonical := range canonicalFieldAliases {
		if _, ok := out[canonical]; ok {
			delete(out, alias)
			continue
		}
		if v, ok := out[alias]; ok {
			out[canonical] = v
			delete(out, alias)
		}
	}

	return out
}

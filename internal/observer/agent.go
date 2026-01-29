package observer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Backend interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

type Tool interface {
	Name() string
	Description() string
	Run(ctx context.Context, args map[string]any) (any, error)
}

type Agent struct {
	LLM          Backend
	Tools        map[string]Tool
	MaxSteps     int
	SystemPrompt string

	OnToolCall   func(tool string, args map[string]any)
	OnToolResult func(tool string, result any)
}

const (
	ErrNoLLM            = "no_llm"
	ErrInvalidModelJSON = "invalid_model_json"
	ErrUnknownTool      = "unknown_tool"
	ErrToolFailed       = "tool_failed"
	ErrMaxStepsExceeded = "max_steps_exceeded"
)

type AgentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e AgentError) Error() string {
	if strings.TrimSpace(e.Detail) != "" {
		return fmt.Sprintf("agent: %s: %s (%s)", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("agent: %s: %s", e.Code, e.Message)
}

func AsAgentError(err error, target *AgentError) bool {
	return errors.As(err, target)
}

type agentStep struct {
	Action  string         `json:"action"`
	Tool    string         `json:"tool,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
	Message string         `json:"message,omitempty"`
}

func (a Agent) Run(ctx context.Context, userMessage string) (string, error) {
	if a.LLM == nil {
		return "", AgentError{Code: ErrNoLLM, Message: "LLM backend is not configured"}
	}

	maxSteps := a.MaxSteps
	if maxSteps <= 0 || maxSteps > 32 {
		maxSteps = 8
	}

	sys := strings.TrimSpace(a.SystemPrompt)
	if sys == "" {
		sys = defaultAgentSystemPrompt
	}

	var traces []string
	for step := 0; step < maxSteps; step++ {
		prompt := a.buildPrompt(sys, userMessage, traces)
		raw, err := a.LLM.Complete(ctx, prompt)
		if err != nil {
			return "", err
		}

		parsed, ok := parseAgentStep(raw)
		if !ok {
			// Best-effort fallback: treat as a direct answer.
			return strings.TrimSpace(raw), nil
		}

		switch parsed.Action {
		case "final":
			if strings.TrimSpace(parsed.Message) == "" {
				return "", AgentError{Code: ErrInvalidModelJSON, Message: "missing final.message"}
			}
			return strings.TrimSpace(parsed.Message), nil
		case "tool":
			toolName := strings.TrimSpace(parsed.Tool)
			if toolName == "" {
				return "", AgentError{Code: ErrInvalidModelJSON, Message: "missing tool name"}
			}
			tool := a.Tools[toolName]
			if tool == nil {
				return "", AgentError{Code: ErrUnknownTool, Message: "unknown tool", Detail: toolName}
			}
			args := parsed.Args
			if args == nil {
				args = map[string]any{}
			}

			if a.OnToolCall != nil {
				a.OnToolCall(toolName, args)
			}

			res, err := tool.Run(ctx, args)
			if err != nil {
				return "", AgentError{Code: ErrToolFailed, Message: "tool call failed", Detail: err.Error()}
			}

			if a.OnToolResult != nil {
				a.OnToolResult(toolName, res)
			}

			stepJSON, _ := json.Marshal(parsed)
			resJSON, _ := json.Marshal(res)
			traces = append(traces,
				fmt.Sprintf("TOOL_CALL %s", string(stepJSON)),
				fmt.Sprintf("TOOL_RESULT %s", string(resJSON)),
			)
		default:
			return "", AgentError{Code: ErrInvalidModelJSON, Message: "unknown action", Detail: parsed.Action}
		}
	}

	return "", AgentError{
		Code:    ErrMaxStepsExceeded,
		Message: "agent exceeded max steps",
		Detail:  fmt.Sprintf("max_steps=%d", maxSteps),
	}
}

const defaultAgentSystemPrompt = `You are the ControlCCX Secretary (an agent).

You MUST answer user questions by calling the provided tools when needed, and you MUST NOT invent task/log/system data.

You MUST respond in ONE of the following formats and nothing else.
Prefer the tag format (it is more robust for long messages).

Format A (preferred, tag format):
1) Tool call:
<action>tool</action>
<tool><tool_name></tool>
<args>{...json...}</args>

2) Final answer:
<action>final</action>
<message><your answer in Chinese></message>

IMPORTANT: Do NOT include literal "<action>" or "<message>" inside <message>.

Format B (legacy JSON format):
1) Tool call:
{"action":"tool","tool":"<tool_name>","args":{...}}
2) Final answer:
{"action":"final","message":"<your answer in Chinese>"}`

func (a Agent) buildPrompt(systemPrompt string, userMessage string, traces []string) string {
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\nAvailable tools:\n")
	if len(a.Tools) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		names := make([]string, 0, len(a.Tools))
		for name := range a.Tools {
			names = append(names, name)
		}
		// Keep ordering stable enough for humans; lexical sort.
		// (Not required for correctness, but helps prompt determinism.)
		sortStrings(names)
		for _, name := range names {
			tool := a.Tools[name]
			desc := ""
			if tool != nil {
				desc = strings.TrimSpace(tool.Description())
			}
			if desc == "" {
				desc = "(no description)"
			}
			sb.WriteString("- ")
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(desc)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nUser question:\n")
	sb.WriteString(userMessage)
	sb.WriteString("\n")

	if len(traces) > 0 {
		sb.WriteString("\nPrevious tool interactions (in order):\n")
		for _, t := range traces {
			sb.WriteString(t)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nNow decide the next action.\n")
	return sb.String()
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}

func parseAgentStep(raw string) (agentStep, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return agentStep{}, false
	}

	// Strip code fences if present.
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		// Remove optional language (e.g. json).
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			head := strings.TrimSpace(s[:i])
			if len(head) <= 16 && !strings.ContainsAny(head, "{}[]\"") {
				s = strings.TrimSpace(s[i+1:])
			}
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = strings.TrimSpace(s[:j])
		}
	}

	// Prefer robust tag format for long messages:
	// <action>final</action><message>...</message>
	// <action>tool</action><tool>...</tool><args>{...}</args>
	if step, ok := parseAgentStepTags(s); ok {
		return step, true
	}

	if step, ok := parseAgentStepJSON(s); ok {
		return step, true
	}
	// If the model wrapped JSON with extra text, try extracting from the last json-ish prefix.
	if i := strings.LastIndex(s, `{"action"`); i >= 0 {
		if step, ok := parseAgentStepJSON(s[i:]); ok {
			return step, true
		}
	}
	// Fall back to first object.
	if i := strings.IndexByte(s, '{'); i >= 0 {
		if step, ok := parseAgentStepJSON(s[i:]); ok {
			return step, true
		}
	}
	return agentStep{}, false
}

func parseAgentStepJSON(s string) (agentStep, bool) {
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(s)))
	var step agentStep
	if err := dec.Decode(&step); err != nil {
		return agentStep{}, false
	}
	step.Action = strings.TrimSpace(step.Action)
	step.Tool = strings.TrimSpace(step.Tool)
	step.Message = strings.TrimSpace(step.Message)
	return step, step.Action != ""
}

func parseAgentStepTags(s string) (agentStep, bool) {
	action, ok := extractLastTag(s, "action")
	if !ok {
		return agentStep{}, false
	}
	step := agentStep{Action: strings.TrimSpace(action)}
	if step.Action == "" {
		return agentStep{}, false
	}
	switch step.Action {
	case "final":
		msg, ok := extractLastTag(s, "message")
		if !ok {
			return agentStep{}, false
		}
		step.Message = strings.TrimSpace(msg)
		return step, step.Message != ""
	case "tool":
		tool, ok := extractLastTag(s, "tool")
		if !ok {
			return agentStep{}, false
		}
		step.Tool = strings.TrimSpace(tool)
		if step.Tool == "" {
			return agentStep{}, false
		}
		argsRaw, ok := extractLastTag(s, "args")
		if ok {
			var args map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(argsRaw)), &args); err == nil && args != nil {
				step.Args = args
			}
		}
		return step, true
	default:
		return agentStep{}, false
	}
}

func extractLastTag(s string, tag string) (string, bool) {
	raw := s
	low := strings.ToLower(raw)
	open := "<" + strings.ToLower(tag) + ">"
	close := "</" + strings.ToLower(tag) + ">"

	searchFrom := len(low)
	for searchFrom > 0 {
		i := strings.LastIndex(low[:searchFrom], open)
		if i < 0 {
			return "", false
		}
		start := i + len(open)
		j := strings.Index(low[start:], close)
		if j < 0 {
			searchFrom = i
			continue
		}
		return raw[start : start+j], true
	}
	return "", false
}

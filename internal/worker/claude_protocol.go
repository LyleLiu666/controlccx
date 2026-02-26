package worker

import (
	"bufio"
	"controlccx/internal/tasks"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type claudeControlRequestEnvelope struct {
	Type      string               `json:"type"`
	RequestID string               `json:"request_id"`
	Request   claudeControlRequest `json:"request"`
}

type claudeControlRequest struct {
	Subtype               string          `json:"subtype"`
	ToolName              string          `json:"tool_name,omitempty"`
	Input                 json.RawMessage `json:"input,omitempty"`
	PermissionSuggestions json.RawMessage `json:"permission_suggestions,omitempty"`
	BlockedPaths          string          `json:"blocked_paths,omitempty"`
	ToolUseID             string          `json:"tool_use_id,omitempty"`
	CallbackID            string          `json:"callback_id,omitempty"`
}

type claudeControlResponseMessage struct {
	Type     string                `json:"type"`
	Response claudeControlResponse `json:"response"`
}

type claudeControlResponse struct {
	Subtype   string `json:"subtype"`
	RequestID string `json:"request_id"`
	Response  any    `json:"response,omitempty"`
	Error     string `json:"error,omitempty"`
}

type claudePermissionResult struct {
	Behavior           string          `json:"behavior"`
	UpdatedInput       json.RawMessage `json:"updatedInput,omitempty"`
	UpdatedPermissions any             `json:"updatedPermissions,omitempty"`

	Message   string `json:"message,omitempty"`
	Interrupt *bool  `json:"interrupt,omitempty"`
}

type claudeSDKControlRequest struct {
	Type      string                      `json:"type"`
	RequestID string                      `json:"request_id"`
	Request   claudeSDKControlRequestBody `json:"request"`
}

type claudeSDKControlRequestBody struct {
	Subtype string `json:"subtype"`
	Mode    string `json:"mode,omitempty"`
	Hooks   any    `json:"hooks,omitempty"`
}

type claudeUserMessageEnvelope struct {
	Type    string            `json:"type"`
	Message claudeUserMessage `json:"message"`
}

type claudeUserMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeProtocolPeer struct {
	mu sync.Mutex
	w  *bufio.Writer
	in io.WriteCloser
}

func newClaudeProtocolPeer(stdin io.WriteCloser) *claudeProtocolPeer {
	if stdin == nil {
		return nil
	}
	return &claudeProtocolPeer{
		w:  bufio.NewWriter(stdin),
		in: stdin,
	}
}

func (p *claudeProtocolPeer) sendLine(v any) error {
	if p == nil {
		return fmt.Errorf("claude protocol: stdin not configured")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.w == nil || p.in == nil {
		return fmt.Errorf("claude protocol: stdin not configured")
	}
	if _, err := p.w.Write(raw); err != nil {
		return err
	}
	if err := p.w.WriteByte('\n'); err != nil {
		return err
	}
	return p.w.Flush()
}

func (p *claudeProtocolPeer) CloseStdin() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.in == nil {
		return nil
	}
	if p.w != nil {
		_ = p.w.Flush()
	}
	err := p.in.Close()
	p.in = nil
	p.w = nil
	return err
}

func (p *claudeProtocolPeer) SendInitialize(requestID string, hooks any) error {
	return p.sendLine(claudeSDKControlRequest{
		Type:      "control_request",
		RequestID: strings.TrimSpace(requestID),
		Request: claudeSDKControlRequestBody{
			Subtype: "initialize",
			Hooks:   hooks,
		},
	})
}

func (p *claudeProtocolPeer) SendSetPermissionMode(requestID string, mode string) error {
	return p.sendLine(claudeSDKControlRequest{
		Type:      "control_request",
		RequestID: strings.TrimSpace(requestID),
		Request: claudeSDKControlRequestBody{
			Subtype: "set_permission_mode",
			Mode:    strings.TrimSpace(mode),
		},
	})
}

func (p *claudeProtocolPeer) SendInterrupt(requestID string) error {
	return p.sendLine(claudeSDKControlRequest{
		Type:      "control_request",
		RequestID: strings.TrimSpace(requestID),
		Request: claudeSDKControlRequestBody{
			Subtype: "interrupt",
		},
	})
}

func (p *claudeProtocolPeer) SendUserMessage(content string) error {
	return p.sendLine(claudeUserMessageEnvelope{
		Type: "user",
		Message: claudeUserMessage{
			Role:    "user",
			Content: content,
		},
	})
}

func (p *claudeProtocolPeer) SendControlResponseSuccess(requestID string, response any) error {
	return p.sendLine(claudeControlResponseMessage{
		Type: "control_response",
		Response: claudeControlResponse{
			Subtype:   "success",
			RequestID: strings.TrimSpace(requestID),
			Response:  response,
		},
	})
}

func (p *claudeProtocolPeer) SendControlResponseError(requestID string, errMsg string) error {
	return p.sendLine(claudeControlResponseMessage{
		Type: "control_response",
		Response: claudeControlResponse{
			Subtype:   "error",
			RequestID: strings.TrimSpace(requestID),
			Error:     strings.TrimSpace(errMsg),
		},
	})
}

func normalizeClaudePermissionMode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "default"
	}
	lower := strings.ToLower(raw)
	switch lower {
	case "default":
		return "default"
	case "acceptedits", "accept_edits", "accept-edits":
		return "acceptEdits"
	case "plan":
		return "plan"
	case "bypasspermissions", "bypass_permissions", "bypass-permissions":
		return "bypassPermissions"
	default:
		return "default"
	}
}

type claudeProtocolHandler struct {
	peer         *claudeProtocolPeer
	toolUseNames map[string]string
}

func newClaudeProtocolHandler(peer *claudeProtocolPeer) ProtocolHandler {
	return &claudeProtocolHandler{
		peer:         peer,
		toolUseNames: make(map[string]string),
	}
}

func (h *claudeProtocolHandler) ConsumeStdout(pctx ProtocolContext, r io.Reader) error {
	reader := newLineReader(r)
	for {
		line, tooLong, err := readLineWithLimit(reader, defaultJSONLineMaxBytes)
		if err != nil {
			if isEOF(err) {
				return nil
			}
			pctx.AppendLog(tasks.LogSystem, formatReadError(err).Error())
			return err
		}
		if tooLong {
			pctx.AppendLog(tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}

		pctx.AppendLog(tasks.LogStdout, string(line))
		pctx.SetResumeFailure(string(line))

		if h.peer == nil {
			pctx.SetBlocked(string(line))
		} else {
			h.handleControlRequest(pctx, line)
		}

		parsed, err := parseClaudeJSONLine(line)
		if err == nil {
			for _, u := range parsed.ToolUses {
				id := strings.TrimSpace(u.ID)
				if id != "" {
					h.toolUseNames[id] = strings.TrimSpace(u.Name)
				}
			}
			for _, res := range parsed.ToolResults {
				if !res.IsError {
					continue
				}
				pctx.MarkToolError()
				id := strings.TrimSpace(res.ToolUseID)
				toolName := strings.TrimSpace(h.toolUseNames[id])
				if toolName == "" {
					toolName = id
				}
				if toolName == "" {
					toolName = "unknown"
				}

				exitPart := "exit=?"
				if code, ok := parseToolResultExitCode(res.Content); ok {
					exitPart = fmt.Sprintf("exit=%d", code)
				}
				summary := summarizeToolResultContent(res.Content, 500)
				msg := strings.TrimSpace(fmt.Sprintf("tool_error: %s tool_use_id=%s %s %s", toolName, id, exitPart, summary))
				pctx.AppendLog(tasks.LogStderr, msg)
			}
			if parsed.SessionID != "" {
				pctx.SetSessionID(parsed.SessionID)
			}
			if parsed.AssistantText != "" {
				pctx.AppendLog(tasks.LogAssistant, parsed.AssistantText)
			}
			if parsed.IsResult && h.peer != nil {
				_ = h.peer.CloseStdin()
			}
		}
	}
}

func (h *claudeProtocolHandler) ConsumeStderr(pctx ProtocolContext, r io.Reader) error {
	reader := newLineReader(r)
	for {
		line, tooLong, err := readLineWithLimit(reader, defaultJSONLineMaxBytes)
		if err != nil {
			if isEOF(err) {
				return nil
			}
			pctx.AppendLog(tasks.LogSystem, formatReadError(err).Error())
			return err
		}
		if tooLong {
			pctx.AppendLog(tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}

		sLine := string(line)
		pctx.AppendLog(tasks.LogStderr, sLine)
		pctx.SetResumeFailure(sLine)

		if h.peer == nil {
			pctx.SetBlocked(sLine)
		}
	}
}

func (h *claudeProtocolHandler) CloseStdin() error {
	if h.peer != nil {
		return h.peer.CloseStdin()
	}
	return nil
}

func (h *claudeProtocolHandler) handleControlRequest(pctx ProtocolContext, line []byte) {
	var env claudeControlRequestEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	if strings.TrimSpace(env.Type) != "control_request" {
		return
	}
	requestID := strings.TrimSpace(env.RequestID)
	if requestID == "" {
		return
	}

	switch strings.TrimSpace(env.Request.Subtype) {
	case "can_use_tool":
		toolName := strings.TrimSpace(env.Request.ToolName)
		var inputMap map[string]any
		_ = json.Unmarshal(env.Request.Input, &inputMap)

		result, err := pctx.OnClaudeCanUseTool(toolName, inputMap, env.Request.PermissionSuggestions, env.Request.ToolUseID)
		if err != nil {
			_ = h.peer.SendControlResponseError(requestID, err.Error())
			return
		}
		_ = h.peer.SendControlResponseSuccess(requestID, result)
	case "hook_callback":
		_ = h.peer.SendControlResponseSuccess(requestID, map[string]any{})
	default:
		_ = h.peer.SendControlResponseSuccess(requestID, map[string]any{})
	}
}

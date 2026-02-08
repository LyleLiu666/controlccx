package worker

import (
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
	w  io.Writer
}

func newClaudeProtocolPeer(w io.Writer) *claudeProtocolPeer {
	if w == nil {
		return nil
	}
	return &claudeProtocolPeer{w: w}
}

func (p *claudeProtocolPeer) sendLine(v any) error {
	if p == nil || p.w == nil {
		return fmt.Errorf("claude protocol: stdin not configured")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.w.Write(raw); err != nil {
		return err
	}
	if _, err := p.w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
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

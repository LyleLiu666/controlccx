package worker

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"controlccx/internal/tasks"
)

// managerProtocolContext bridges the generic ProtocolContext interface back to Manager's state.
type managerProtocolContext struct {
	m                  *Manager
	ctx                context.Context
	task               tasks.Task
	driver             tasks.WorkerType
	runWorkspaceActive bool
	initProject        bool

	sidMu         *sync.Mutex
	sid           *string
	cancel        context.CancelFunc
	resumeFailure *resumeFailureState
	blocked       *blockedState
	toolErrors    *toolErrorState

	codexPeer *codexAppServerPeer
}

func (p *managerProtocolContext) Task() tasks.Task {
	return p.task
}

func (p *managerProtocolContext) Context() context.Context {
	return p.ctx
}

func (p *managerProtocolContext) AppendLog(stream tasks.LogStream, message string) {
	p.m.appendLog(p.task.ID, stream, message)
}

func (p *managerProtocolContext) SetSessionID(sid string) {
	shouldPublish := false
	p.sidMu.Lock()
	if *p.sid == "" {
		*p.sid = sid
		shouldPublish = true
	}
	p.sidMu.Unlock()

	_ = p.m.store.SetSessionID(p.ctx, p.task.ID, sid)
	if shouldPublish {
		p.m.publishTaskUpdatedForce(p.task.ID)
	}
}

func (p *managerProtocolContext) PublishTaskUpdated() {
	p.m.publishTaskUpdatedForce(p.task.ID)
}

func (p *managerProtocolContext) SetResumeFailure(msg string) {
	p.m.handleResumeNotFound(p.task, p.driver, msg, p.cancel, p.resumeFailure)
}

func (p *managerProtocolContext) SetBlocked(reason string) {
	p.m.handleApprovalRequired(p.task, p.driver, reason, []byte(reason), p.blocked)
}

func (p *managerProtocolContext) MarkToolError() {
	p.toolErrors.mark()
}

func (p *managerProtocolContext) OnClaudeCanUseTool(toolName string, input map[string]any, suggestions []byte, toolUseID string) (any, error) {
	rawInput, err := json.Marshal(input)
	if err != nil {
		rawInput = nil
	}
	return p.m.onClaudeCanUseTool(p.ctx, p.task, toolName, rawInput, suggestions, toolUseID, p.runWorkspaceActive, p.initProject, p.blocked)
}

func (p *managerProtocolContext) OnCodexServerRequest(id []byte, method string, params []byte) {
	if p.codexPeer != nil {
		p.m.handleCodexServerRequest(p.ctx, p.task, p.codexPeer, id, method, params, p.runWorkspaceActive, p.initProject, p.blocked)
	}
}

type defaultProtocolHandler struct{}

func newDefaultProtocolHandler() ProtocolHandler {
	return &defaultProtocolHandler{}
}

func (h *defaultProtocolHandler) ConsumeStdout(pctx ProtocolContext, r io.Reader) error {
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
	}
}

func (h *defaultProtocolHandler) ConsumeStderr(pctx ProtocolContext, r io.Reader) error {
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
		pctx.AppendLog(tasks.LogStderr, string(line))
		pctx.SetResumeFailure(string(line))
	}
}

func (h *defaultProtocolHandler) CloseStdin() error {
	return nil
}

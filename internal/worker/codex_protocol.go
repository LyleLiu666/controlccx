package worker

import (
	"bufio"
	"context"
	"controlccx/internal/tasks"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type codexJSONRPCError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type codexJSONRPCEnvelope struct {
	JSONRPC string             `json:"jsonrpc,omitempty"`
	ID      json.RawMessage    `json:"id,omitempty"`
	Method  string             `json:"method,omitempty"`
	Params  json.RawMessage    `json:"params,omitempty"`
	Result  json.RawMessage    `json:"result,omitempty"`
	Error   *codexJSONRPCError `json:"error,omitempty"`
}

type codexPendingResult struct {
	Result json.RawMessage
	Err    error
}

type codexAppServerPeer struct {
	stdin io.WriteCloser
	w     *bufio.Writer

	writeMu sync.Mutex

	nextID int64

	pendingMu sync.Mutex
	pending   map[string]chan codexPendingResult

	doneOnce sync.Once
	doneCh   chan struct{}
	doneErr  atomic.Value // string
}

func newCodexAppServerPeer(stdin io.WriteCloser) *codexAppServerPeer {
	return &codexAppServerPeer{
		stdin:   stdin,
		w:       bufio.NewWriter(stdin),
		pending: make(map[string]chan codexPendingResult),
		doneCh:  make(chan struct{}),
	}
}

func (p *codexAppServerPeer) CloseStdin() error {
	if p == nil {
		return nil
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.stdin == nil {
		return nil
	}
	_ = p.w.Flush()
	err := p.stdin.Close()
	p.stdin = nil
	p.w = nil
	return err
}

func (p *codexAppServerPeer) signalDone(errText string) {
	if p == nil {
		return
	}
	p.doneOnce.Do(func() {
		p.doneErr.Store(strings.TrimSpace(errText))
		close(p.doneCh)
	})
}

func (p *codexAppServerPeer) WaitDone(ctx context.Context) error {
	if p == nil {
		return nil
	}
	select {
	case <-p.doneCh:
		if v := p.doneErr.Load(); v != nil {
			if msg := strings.TrimSpace(v.(string)); msg != "" {
				return errors.New(msg)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *codexAppServerPeer) nextRequestID() int64 {
	return atomic.AddInt64(&p.nextID, 1)
}

func (p *codexAppServerPeer) send(payload any) error {
	if p == nil {
		return errors.New("codex peer is nil")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.w == nil {
		return errors.New("codex peer stdin closed")
	}
	if _, err := p.w.Write(b); err != nil {
		return err
	}
	if err := p.w.WriteByte('\n'); err != nil {
		return err
	}
	return p.w.Flush()
}

func (p *codexAppServerPeer) Notify(method string, params any) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return errors.New("codex notify method is required")
	}
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return p.send(msg)
}

func (p *codexAppServerPeer) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, errors.New("codex request method is required")
	}
	if ctx == nil {
		return nil, errors.New("codex request ctx is required")
	}

	id := p.nextRequestID()
	key := strconv.FormatInt(id, 10)
	ch := make(chan codexPendingResult, 1)

	p.pendingMu.Lock()
	if p.pending == nil {
		p.pending = make(map[string]chan codexPendingResult)
	}
	p.pending[key] = ch
	p.pendingMu.Unlock()

	defer func() {
		p.pendingMu.Lock()
		delete(p.pending, key)
		p.pendingMu.Unlock()
	}()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := p.send(msg); err != nil {
		return nil, err
	}

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *codexAppServerPeer) SendResult(id json.RawMessage, result any) error {
	if p == nil {
		return errors.New("codex peer is nil")
	}
	if len(id) == 0 {
		return errors.New("codex response id is required")
	}
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	return p.send(msg)
}

func (p *codexAppServerPeer) SendError(id json.RawMessage, code int64, message string) error {
	if p == nil {
		return errors.New("codex peer is nil")
	}
	if len(id) == 0 {
		return errors.New("codex error id is required")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "unknown error"
	}
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	return p.send(msg)
}

func codexIDKey(id json.RawMessage) (string, bool) {
	if len(id) == 0 {
		return "", false
	}
	var v any
	if err := json.Unmarshal(id, &v); err != nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return "", false
		}
		return t, true
	case float64:
		return strconv.FormatInt(int64(t), 10), true
	default:
		return "", false
	}
}

func (p *codexAppServerPeer) resolve(id json.RawMessage, res codexPendingResult) {
	if p == nil {
		return
	}
	key, ok := codexIDKey(id)
	if !ok {
		return
	}
	p.pendingMu.Lock()
	ch := p.pending[key]
	p.pendingMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

func (p *codexAppServerPeer) HandleLine(line []byte, onRequest func(id json.RawMessage, method string, params json.RawMessage), onNotification func(method string, params json.RawMessage)) {
	if p == nil {
		return
	}
	var env codexJSONRPCEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}

	method := strings.TrimSpace(env.Method)
	if method == "" && len(env.ID) != 0 {
		// Response or error for a client-initiated request.
		if env.Error != nil {
			p.resolve(env.ID, codexPendingResult{
				Err: fmt.Errorf("codex rpc error (%d): %s", env.Error.Code, strings.TrimSpace(env.Error.Message)),
			})
			return
		}
		p.resolve(env.ID, codexPendingResult{Result: env.Result, Err: nil})
		return
	}

	// Server notification or server-initiated request.
	if method == "" {
		return
	}
	if method == "turn/completed" {
		p.signalDone("")
	} else if method == "error" {
		// Best-effort: surface the first error as the turn failure reason.
		var payload struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(env.Params, &payload); err == nil {
			p.signalDone(payload.Message)
		} else {
			p.signalDone("codex error")
		}
	}

	if len(env.ID) != 0 {
		if onRequest != nil {
			onRequest(env.ID, method, env.Params)
		}
		return
	}
	if onNotification != nil {
		onNotification(method, env.Params)
	}
}

type codexProtocolHandler struct {
	peer *codexAppServerPeer
}

func newCodexProtocolHandler(peer *codexAppServerPeer) ProtocolHandler {
	return &codexProtocolHandler{
		peer: peer,
	}
}

func (h *codexProtocolHandler) ConsumeStdout(pctx ProtocolContext, r io.Reader) error {
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
			h.peer.HandleLine(line, func(id json.RawMessage, method string, params json.RawMessage) {
				go pctx.OnCodexServerRequest(id, method, params)
			}, nil)
		}

		parsed, err := parseCodexJSONLine(line)
		if err == nil {
			if parsed.SessionID != "" {
				pctx.SetSessionID(parsed.SessionID)
			}
			if parsed.AssistantText != "" {
				pctx.AppendLog(tasks.LogAssistant, parsed.AssistantText)
			}
		}
	}
}

func (h *codexProtocolHandler) ConsumeStderr(pctx ProtocolContext, r io.Reader) error {
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

func (h *codexProtocolHandler) CloseStdin() error {
	if h.peer != nil {
		return h.peer.CloseStdin()
	}
	return nil
}

package observer

import (
	"context"
	"encoding/json"
	"testing"
)

type stubBackend struct {
	name    string
	outputs []string
	prompts []string
	i       int
}

func (s *stubBackend) Name() string { return s.name }

func (s *stubBackend) Complete(ctx context.Context, prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	if s.i >= len(s.outputs) {
		return `{"action":"final","message":"no more outputs"}`, nil
	}
	out := s.outputs[s.i]
	s.i++
	return out, nil
}

type toolFunc struct {
	name string
	desc string
	run  func(ctx context.Context, args map[string]any) (any, error)
}

func (t toolFunc) Name() string        { return t.name }
func (t toolFunc) Description() string { return t.desc }
func (t toolFunc) Run(ctx context.Context, args map[string]any) (any, error) {
	return t.run(ctx, args)
}

func TestAgent_Run_ToolThenFinal(t *testing.T) {
	ctx := context.Background()

	var gotArgs map[string]any
	add := toolFunc{
		name: "add",
		desc: "add two numbers",
		run: func(ctx context.Context, args map[string]any) (any, error) {
			gotArgs = args
			a := int(args["a"].(float64))
			b := int(args["b"].(float64))
			return map[string]any{"sum": a + b}, nil
		},
	}

	llm := &stubBackend{
		name: "stub",
		outputs: []string{
			`{"action":"tool","tool":"add","args":{"a":1,"b":2}}`,
			`{"action":"final","message":"sum is 3"}`,
		},
	}

	agent := Agent{
		LLM:   llm,
		Tools: map[string]Tool{"add": add},
	}

	msg, err := agent.Run(ctx, "compute 1+2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if msg != "sum is 3" {
		t.Fatalf("msg=%q, want %q", msg, "sum is 3")
	}
	if gotArgs == nil || gotArgs["a"] == nil || gotArgs["b"] == nil {
		t.Fatalf("expected tool args captured, got %+v", gotArgs)
	}
}

func TestAgent_Run_InvalidJSON_ReturnsRaw(t *testing.T) {
 	ctx := context.Background()
 	llm := &stubBackend{name: "stub", outputs: []string{"not json"}}
 	agent := Agent{LLM: llm, Tools: map[string]Tool{}}
 	msg, err := agent.Run(ctx, "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if msg != "not json" {
		t.Fatalf("msg=%q, want raw output", msg)
	}
}

func TestAgent_Run_JSONWithTrailingText_StillParses(t *testing.T) {
	ctx := context.Background()
	llm := &stubBackend{name: "stub", outputs: []string{`{"action":"final","message":"ok"}\nextra`}}
	agent := Agent{LLM: llm, Tools: map[string]Tool{}}
	msg, err := agent.Run(ctx, "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if msg != "ok" {
		t.Fatalf("msg=%q, want %q", msg, "ok")
	}
}

func TestAgent_Run_TagFormat_ToolThenFinal(t *testing.T) {
	ctx := context.Background()

	var got bool
	noop := toolFunc{
		name: "noop",
		desc: "noop",
		run: func(ctx context.Context, args map[string]any) (any, error) {
			got = true
			return map[string]any{"ok": true}, nil
		},
	}

	llm := &stubBackend{
		name: "stub",
		outputs: []string{
			"<action>tool</action>\n<tool>noop</tool>\n<args>{}</args>",
			"<action>final</action>\n<message>done</message>",
		},
	}

	agent := Agent{
		LLM:   llm,
		Tools: map[string]Tool{"noop": noop},
	}

	msg, err := agent.Run(ctx, "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if msg != "done" {
		t.Fatalf("msg=%q, want %q", msg, "done")
	}
	if !got {
		t.Fatalf("expected tool to be called")
	}
}

func TestAgent_Run_MaxStepsExceeded(t *testing.T) {
	ctx := context.Background()
	llm := &stubBackend{
		name: "stub",
		outputs: []string{
			`{"action":"tool","tool":"noop","args":{}}`,
			`{"action":"tool","tool":"noop","args":{}}`,
		},
	}
	agent := Agent{
		LLM:      llm,
		MaxSteps: 1,
		Tools: map[string]Tool{
			"noop": toolFunc{name: "noop", desc: "noop", run: func(ctx context.Context, args map[string]any) (any, error) {
				return map[string]any{"ok": true}, nil
			}},
		},
	}
	_, err := agent.Run(ctx, "loop")
	if err == nil {
		t.Fatalf("expected error")
	}
	var e AgentError
	if !AsAgentError(err, &e) {
		// Don't require exact type; but should still be JSON-ish for debugging.
		return
	}
	if e.Code != ErrMaxStepsExceeded {
		b, _ := json.Marshal(e)
		t.Fatalf("err=%s, want code=%q", string(b), ErrMaxStepsExceeded)
	}
}

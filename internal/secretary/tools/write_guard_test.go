package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
)

type guardWriteTool struct {
	executed *bool
}

func (t guardWriteTool) Name() string { return "task_cancel_submit" }

func (t guardWriteTool) DescriptionZH() string { return "test write tool" }

func (t guardWriteTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	_ = call
	_ = deps
	if t.executed != nil {
		*t.executed = true
	}
	return map[string]any{"ok": true}, nil
}

type guardRecorder struct {
	err   error
	plans []ActionPlan
}

func (r *guardRecorder) RecordActionPlan(ctx context.Context, plan ActionPlan) error {
	_ = ctx
	r.plans = append(r.plans, plan)
	return r.err
}

func TestWriteGuard_GenerationFailureIsFailClosed(t *testing.T) {
	ctx := context.Background()
	executed := false
	mainRecorder := &guardRecorder{}
	reg := newRegistryWithTools(Deps{
		WriteGuardEnabled: true,
		ActionPlanBuilder: func(call agentsdk.ToolCall) (ActionPlan, error) {
			_ = call
			return ActionPlan{}, errors.New("boom-generate")
		},
		ActionPlanMainRecorder: mainRecorder,
	}, []Tool{guardWriteTool{executed: &executed}})

	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "task_cancel_submit",
		Fields: map[string]string{"task_id": "task-a"},
	})
	if err == nil {
		t.Fatalf("expected fail-closed error")
	}
	if !strings.Contains(err.Error(), "action_plan generate failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed {
		t.Fatalf("tool should not execute on generation failure")
	}
	if len(mainRecorder.plans) != 0 {
		t.Fatalf("main recorder should not be called when generation fails")
	}
}

func TestWriteGuard_AuditFailureIsFailClosed(t *testing.T) {
	ctx := context.Background()
	executed := false
	mainRecorder := &guardRecorder{err: errors.New("boom-audit")}
	blockCount := 0
	reg := newRegistryWithTools(Deps{
		WriteGuardEnabled:      true,
		ActionPlanMainRecorder: mainRecorder,
		OnWriteGuardBlock: func(err error) {
			if err != nil {
				blockCount++
			}
		},
	}, []Tool{guardWriteTool{executed: &executed}})

	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "task_cancel_submit",
		Fields: map[string]string{"task_id": "task-a"},
	})
	if err == nil {
		t.Fatalf("expected fail-closed error")
	}
	if !strings.Contains(err.Error(), "action_plan audit record failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed {
		t.Fatalf("tool should not execute on audit failure")
	}
	if len(mainRecorder.plans) != 1 {
		t.Fatalf("main recorder calls=%d want 1", len(mainRecorder.plans))
	}
	if blockCount != 1 {
		t.Fatalf("blockCount=%d want 1", blockCount)
	}
}

func TestWriteGuard_EventFailureIsFailOpen(t *testing.T) {
	ctx := context.Background()
	executed := false
	mainRecorder := &guardRecorder{}
	eventRecorder := &guardRecorder{err: errors.New("boom-event")}
	sideErrCount := 0
	emittedCount := 0

	reg := newRegistryWithTools(Deps{
		WriteGuardEnabled:       true,
		ActionPlanMainRecorder:  mainRecorder,
		ActionPlanEventRecorder: eventRecorder,
		OnActionPlanEmitted: func(plan ActionPlan) {
			if strings.TrimSpace(plan.ID) != "" {
				emittedCount++
			}
		},
		OnWriteGuardSideEffectErr: func(err error) {
			if err != nil {
				sideErrCount++
			}
		},
	}, []Tool{guardWriteTool{executed: &executed}})

	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "task_cancel_submit",
		Fields: map[string]string{"task_id": "task-a"},
	})
	if err != nil {
		t.Fatalf("expected fail-open success, got err=%v", err)
	}
	if !executed {
		t.Fatalf("tool should execute when side-event recorder fails")
	}
	if len(mainRecorder.plans) != 1 {
		t.Fatalf("main recorder calls=%d want 1", len(mainRecorder.plans))
	}
	if len(eventRecorder.plans) != 1 {
		t.Fatalf("event recorder calls=%d want 1", len(eventRecorder.plans))
	}
	if sideErrCount != 1 {
		t.Fatalf("sideErrCount=%d want 1", sideErrCount)
	}
	if emittedCount != 1 {
		t.Fatalf("emittedCount=%d want 1", emittedCount)
	}
}

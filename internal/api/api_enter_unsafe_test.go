package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

type stubRunner struct {
	startCalls  []string
	cancelCalls []string
}

func (s *stubRunner) Start(ctx context.Context, taskID string) error {
	s.startCalls = append(s.startCalls, taskID)
	return nil
}

func (s *stubRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	s.cancelCalls = append(s.cancelCalls, taskID)
	return true, nil
}

func TestAPI_TaskEnterUnsafe_CancelsAndCreatesFollowup(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	runner := &stubRunner{}

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: runner,
		Hub:     hub,
	}

	src, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create src: %v", err)
	}
	if err := taskStore.SetRunning(ctx, src.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	buf, _ := json.Marshal(map[string]any{"prompt": "continue"})
	res, err := http.Post(srv.URL+"/api/tasks/"+src.ID+"/enter-unsafe", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post enter-unsafe: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	bodyOut := decodeMutationResponse(t, res)
	requireMutationAction(t, bodyOut, "task.enter_unsafe")
	outTask := requireMutationTask(t, bodyOut)
	if outTask.ID == "" || outTask.ID == src.ID {
		t.Fatalf("task.id=%q, want new id", outTask.ID)
	}
	if outTask.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q, want %q", outTask.Mode, tasks.ModeResume)
	}
	if outTask.ConversationID != src.ConversationID {
		t.Fatalf("conversation_id=%q, want %q", outTask.ConversationID, src.ConversationID)
	}
	if outTask.SessionID != "sess-1" {
		t.Fatalf("session_id=%q, want %q", outTask.SessionID, "sess-1")
	}
	if !outTask.UnsafeAutomation {
		t.Fatalf("unsafe_automation=false, want true")
	}
	if outTask.SafetyPreset != "unsafe" {
		t.Fatalf("safety_preset=%q, want %q", outTask.SafetyPreset, "unsafe")
	}
	if outTask.WorkDirStrategy != "wait" {
		t.Fatalf("workdir_strategy=%q, want %q", outTask.WorkDirStrategy, "wait")
	}
	if outTask.Status != tasks.StatusWaiting {
		t.Fatalf("status=%q, want %q", outTask.Status, tasks.StatusWaiting)
	}

	proofs, err := taskStore.ListRollbackProofsByAction(ctx, src.ID, "task.enter_unsafe", src.ID, tasks.ListRollbackProofsOptions{
		ProofType: "restore_point",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListRollbackProofsByAction: %v", err)
	}
	if len(proofs) == 0 {
		t.Fatalf("expected restore_point proof for task %s", src.ID)
	}
	if proofs[0].ProofType != "restore_point" {
		t.Fatalf("proof_type=%q, want %q", proofs[0].ProofType, "restore_point")
	}

	decisions, err := taskStore.ListRiskDecisionsByTask(ctx, src.ID, tasks.ListRiskDecisionsOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListRiskDecisionsByTask: %v", err)
	}
	if len(decisions) == 0 {
		t.Fatalf("expected at least one risk decision for task %s", src.ID)
	}
	latest := decisions[0]
	if latest.ActionType != "task.enter_unsafe" {
		t.Fatalf("action_type=%q, want %q", latest.ActionType, "task.enter_unsafe")
	}
	if latest.RiskLevel != tasks.RiskHigh {
		t.Fatalf("risk_level=%q, want %q", latest.RiskLevel, tasks.RiskHigh)
	}
	var scope map[string]any
	if err := json.Unmarshal(latest.Scope, &scope); err != nil {
		t.Fatalf("scope json: %v", err)
	}
	if _, ok := scope["reversible"]; !ok {
		t.Fatalf("scope missing reversible field: %v", scope)
	}
	if _, ok := scope["reversibility"]; !ok {
		t.Fatalf("scope missing reversibility field: %v", scope)
	}

	if len(runner.cancelCalls) != 1 || runner.cancelCalls[0] != src.ID {
		t.Fatalf("cancelCalls=%v, want [%s]", runner.cancelCalls, src.ID)
	}
	if len(runner.startCalls) != 0 {
		t.Fatalf("startCalls=%v, want none (waiting task should not be started)", runner.startCalls)
	}
}

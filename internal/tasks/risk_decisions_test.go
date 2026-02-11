package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestRiskDecisions_CRUDAndQuery(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerClaudeCode,
		Mode:       ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	first, err := store.CreateRiskDecision(ctx, CreateRiskDecisionInput{
		TaskID:         task.ID,
		SessionID:      task.SessionID,
		ConversationID: task.ConversationID,
		WorkerType:     task.WorkerType,
		ActionType:     "run.exec",
		RiskLevel:      RiskHigh,
		Decision:       "review",
		Rationale:      "downloads and executes script",
		Scope:          []byte(`{"files":["setup.sh"],"network":["github.com"]}`),
		Source:         "risk-evaluator-v1",
	})
	if err != nil {
		t.Fatalf("CreateRiskDecision first: %v", err)
	}
	if first.ID == "" {
		t.Fatalf("expected id")
	}
	if first.Decision != "review" {
		t.Fatalf("decision=%q, want %q", first.Decision, "review")
	}

	second, err := store.CreateRiskDecision(ctx, CreateRiskDecisionInput{
		TaskID:         task.ID,
		SessionID:      task.SessionID,
		ConversationID: task.ConversationID,
		WorkerType:     task.WorkerType,
		ActionType:     "run.exec",
		RiskLevel:      RiskMedium,
		Decision:       "allow",
		Rationale:      "read-only and reversible",
		Scope:          []byte(`{"files":["README.md"]}`),
		Source:         "risk-evaluator-v1",
	})
	if err != nil {
		t.Fatalf("CreateRiskDecision second: %v", err)
	}

	byTask, err := store.ListRiskDecisionsByTask(ctx, task.ID, ListRiskDecisionsOptions{})
	if err != nil {
		t.Fatalf("ListRiskDecisionsByTask: %v", err)
	}
	if len(byTask) != 2 {
		t.Fatalf("len(byTask)=%d, want 2", len(byTask))
	}

	byTaskAllowOnly, err := store.ListRiskDecisionsByTask(ctx, task.ID, ListRiskDecisionsOptions{Decision: "allow"})
	if err != nil {
		t.Fatalf("ListRiskDecisionsByTask allow: %v", err)
	}
	if len(byTaskAllowOnly) != 1 || byTaskAllowOnly[0].ID != second.ID {
		t.Fatalf("allow decisions=%v, want only %q", byTaskAllowOnly, second.ID)
	}

	bySession, err := store.ListRiskDecisionsBySession(ctx, task.SessionID, ListRiskDecisionsOptions{})
	if err != nil {
		t.Fatalf("ListRiskDecisionsBySession: %v", err)
	}
	if len(bySession) != 2 {
		t.Fatalf("len(bySession)=%d, want 2", len(bySession))
	}
	if bySession[0].SessionID != task.SessionID {
		t.Fatalf("session_id=%q, want %q", bySession[0].SessionID, task.SessionID)
	}
}

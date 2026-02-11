package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestRollbackProofs_CRUDAndQuery(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerCodex,
		Mode:       ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	first, err := store.CreateRollbackProof(ctx, CreateRollbackProofInput{
		TaskID:     task.ID,
		ActionType: "apply_patch",
		ActionRef:  "it-1",
		ProofType:  "git_commit",
		ProofRef:   "abc123",
		Detail:     []byte(`{"branch":"feat/x"}`),
	})
	if err != nil {
		t.Fatalf("CreateRollbackProof first: %v", err)
	}
	if first.ID == "" {
		t.Fatalf("expected id")
	}
	if first.ProofType != "git_commit" {
		t.Fatalf("proof_type=%q, want %q", first.ProofType, "git_commit")
	}

	second, err := store.CreateRollbackProof(ctx, CreateRollbackProofInput{
		TaskID:     task.ID,
		ActionType: "apply_patch",
		ActionRef:  "it-2",
		ProofType:  "restore_point",
		ProofRef:   "rp-001",
		Detail:     []byte(`{"snapshot":"snap-a"}`),
	})
	if err != nil {
		t.Fatalf("CreateRollbackProof second: %v", err)
	}

	byTask, err := store.ListRollbackProofsByTask(ctx, task.ID, ListRollbackProofsOptions{})
	if err != nil {
		t.Fatalf("ListRollbackProofsByTask: %v", err)
	}
	if len(byTask) != 2 {
		t.Fatalf("len(byTask)=%d, want 2", len(byTask))
	}

	byAction, err := store.ListRollbackProofsByAction(ctx, task.ID, "apply_patch", "it-2", ListRollbackProofsOptions{})
	if err != nil {
		t.Fatalf("ListRollbackProofsByAction: %v", err)
	}
	if len(byAction) != 1 || byAction[0].ID != second.ID {
		t.Fatalf("byAction=%v, want only %q", byAction, second.ID)
	}
}

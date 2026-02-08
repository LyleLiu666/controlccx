package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_ListTasksWithOptions_SortsByLatestExecutionTime(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	now := time.Date(2026, 1, 30, 16, 29, 37, 0, time.UTC)
	store.now = func() time.Time { return now }

	older, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "older",
		WorkDir:    filepath.Join(t.TempDir(), "older"),
	})
	if err != nil {
		t.Fatalf("create older: %v", err)
	}

	now = now.Add(2 * time.Hour)
	newer, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "newer",
		WorkDir:    filepath.Join(t.TempDir(), "newer"),
	})
	if err != nil {
		t.Fatalf("create newer: %v", err)
	}

	now = now.Add(24 * time.Hour)
	if err := store.SetRunning(ctx, older.ID); err != nil {
		t.Fatalf("set running older: %v", err)
	}
	finishAt := now.Add(10 * time.Minute)
	if err := store.FinishTask(ctx, older.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		FinishedAt: finishAt,
	}); err != nil {
		t.Fatalf("finish older: %v", err)
	}

	list, err := store.ListTasksWithOptions(ctx, 10, ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("len(list)=%d, want >=2", len(list))
	}
	if list[0].ID != older.ID {
		t.Fatalf("first id=%q, want older=%q", list[0].ID, older.ID)
	}
	if list[1].ID != newer.ID {
		t.Fatalf("second id=%q, want newer=%q", list[1].ID, newer.ID)
	}
}

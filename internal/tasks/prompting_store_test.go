package tasks

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
)

func TestStore_ProjectContext_SetAndGet(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	_, ok, err := store.GetProjectContext(ctx)
	if err != nil {
		t.Fatalf("GetProjectContext: %v", err)
	}
	if ok {
		t.Fatalf("expected no context initially")
	}

	got, err := store.SetProjectContext(ctx, "  hello \n")
	if err != nil {
		t.Fatalf("SetProjectContext: %v", err)
	}
	if strings.TrimSpace(got.Content) != "hello" {
		t.Fatalf("content=%q, want %q", got.Content, "hello")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at set")
	}

	read, ok, err := store.GetProjectContext(ctx)
	if err != nil {
		t.Fatalf("GetProjectContext: %v", err)
	}
	if !ok {
		t.Fatalf("expected context present")
	}
	if read.Content != got.Content {
		t.Fatalf("content=%q, want %q", read.Content, got.Content)
	}
}

func TestStore_PromptTemplates_CRUD(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	created, err := store.UpsertPromptTemplate(ctx, UpsertPromptTemplateInput{
		Title:   "Task template",
		Kind:    "task",
		Content: "do the thing",
	})
	if err != nil {
		t.Fatalf("UpsertPromptTemplate: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatalf("expected template id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps set: %+v", created)
	}

	list, err := store.ListPromptTemplates(ctx, "task")
	if err != nil {
		t.Fatalf("ListPromptTemplates: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list=%+v, want 1 created template", list)
	}

	updated, err := store.UpsertPromptTemplate(ctx, UpsertPromptTemplateInput{
		ID:      created.ID,
		Title:   "Task template v2",
		Kind:    "task",
		Content: "do the thing better",
	})
	if err != nil {
		t.Fatalf("UpsertPromptTemplate: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("id=%q, want %q", updated.ID, created.ID)
	}
	if updated.Title != "Task template v2" || updated.Content != "do the thing better" {
		t.Fatalf("updated=%+v, want updated fields", updated)
	}

	if err := store.DeletePromptTemplate(ctx, created.ID); err != nil {
		t.Fatalf("DeletePromptTemplate: %v", err)
	}
	list2, err := store.ListPromptTemplates(ctx, "task")
	if err != nil {
		t.Fatalf("ListPromptTemplates: %v", err)
	}
	if len(list2) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", list2)
	}
}

package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionsService_ListCreateDelete(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "skill-a"))
	mustWrite(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "a\n")

	svc, err := NewVersionsService(VersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 1, 30, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new versions service: %v", err)
	}

	before, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(before.Versions) != 0 {
		t.Fatalf("versions=%v", before.Versions)
	}

	v1, err := svc.Create(ctx, CreateVersionInput{ID: "20260130-01"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v1.ID != "20260130-01" {
		t.Fatalf("id=%q", v1.ID)
	}
	wantPath := filepath.Join(home, ".agent", "skills_versions", "20260130-01")
	if _, err := os.Stat(filepath.Join(wantPath, "skill-a", "README.md")); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}

	// Duplicate should fail.
	if _, err := svc.Create(ctx, CreateVersionInput{ID: "20260130-01"}); err == nil {
		t.Fatalf("expected duplicate create to fail")
	}

	// Auto-generate should pick next slot for the day.
	v2, err := svc.Create(ctx, CreateVersionInput{})
	if err != nil {
		t.Fatalf("create auto: %v", err)
	}
	if v2.ID != "20260130-02" {
		t.Fatalf("auto id=%q", v2.ID)
	}

	out, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(out.Versions) != 2 {
		t.Fatalf("versions=%v", out.Versions)
	}

	if err := svc.Delete(ctx, "20260130-01"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted, err=%v", err)
	}
}

package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestService_InstallLocal_AndUpdateFromSource(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")

	localSrc := filepath.Join(home, "src-skill")
	mustMkdir(t, localSrc)
	mustWrite(t, filepath.Join(localSrc, "SKILL.md"), "hi\n")

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.InstallLocal(ctx, InstallLocalInput{
		SourcePath: localSrc,
		Name:       "demo",
	})
	if err != nil {
		t.Fatalf("install local: %v", err)
	}
	if created.Name != "demo" {
		t.Fatalf("name=%q", created.Name)
	}
	destDir := filepath.Join(sourceRoot, "demo")
	if _, err := os.Stat(destDir); err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, managedManifestFile)); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}

	// Update source and ensure update applies.
	mustWrite(t, filepath.Join(localSrc, "SKILL.md"), "updated\n")

	updated, err := svc.UpdateManagedSkill(ctx, "demo")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "updated\n" {
		t.Fatalf("dest content=%q", string(got))
	}
	if updated.ContentHash == "" {
		t.Fatalf("expected content hash")
	}
}

func TestService_InstallLocal_TargetExists_ErrorPrefix(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "demo"))

	localSrc := filepath.Join(home, "src-skill")
	mustMkdir(t, localSrc)

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.InstallLocal(ctx, InstallLocalInput{SourcePath: localSrc, Name: "demo"})
	if err == nil || !hasPrefix(err.Error(), errPrefixTargetExists) {
		t.Fatalf("expected %s error, got=%v", errPrefixTargetExists, err)
	}
}

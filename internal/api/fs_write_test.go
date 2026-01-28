package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSWrite_WritesFileUnderBase(t *testing.T) {
	root := t.TempDir()
	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}

	body := map[string]any{
		"base":    root,
		"path":    "a.txt",
		"content": "hello",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/fs/write", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content=%q, want %q", string(data), "hello")
	}
}

func TestFSWrite_RejectsOversizeContent(t *testing.T) {
	root := t.TempDir()
	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}

	tooBig := strings.Repeat("a", (1<<20)+1)
	body := map[string]any{
		"base":    root,
		"path":    "big.txt",
		"content": tooBig,
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/fs/write", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "big.txt")); err == nil {
		t.Fatalf("expected file not to be created")
	}
}

func TestFSWrite_BlocksTraversalOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}

	body := map[string]any{
		"base":    root,
		"path":    filepath.Join("..", filepath.Base(outside), "x.txt"),
		"content": "nope",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/fs/write", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

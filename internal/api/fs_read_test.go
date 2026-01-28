package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleFSRead_ResolvesBaseAndReadsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}
	u := "/api/fs/read?base=" + url.QueryEscape(root) + "&path=" + url.QueryEscape("a.txt")
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rr := httptest.NewRecorder()

	a.handleFSRead(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if filepath.Clean(resp.Path) != filepath.Join(root, "a.txt") {
		t.Fatalf("path=%q, want %q", resp.Path, filepath.Join(root, "a.txt"))
	}
	if resp.Content != "hello" {
		t.Fatalf("content=%q, want %q", resp.Content, "hello")
	}
}

func TestHandleFSRead_BlocksTraversalOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}
	u := "/api/fs/read?base=" + url.QueryEscape(root) + "&path=" + url.QueryEscape(filepath.Join("..", filepath.Base(outside), "x.txt"))
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rr := httptest.NewRecorder()

	a.handleFSRead(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

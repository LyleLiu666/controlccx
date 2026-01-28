package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestFSEntries_ListsDirsFirstThenFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}
	u := "/api/fs/entries?base=" + url.QueryEscape(root) + "&path=" + url.QueryEscape(".")
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Entries []FSEntry `json:"entries"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) != 3 {
		t.Fatalf("entries=%d, want 3", len(resp.Entries))
	}
	if resp.Entries[0].Name != "adir" || resp.Entries[0].Kind != FSEntryDir {
		t.Fatalf("0=%+v, want adir dir", resp.Entries[0])
	}
	if resp.Entries[1].Name != "bdir" || resp.Entries[1].Kind != FSEntryDir {
		t.Fatalf("1=%+v, want bdir dir", resp.Entries[1])
	}
	if resp.Entries[2].Name != "c.txt" || resp.Entries[2].Kind != FSEntryFile {
		t.Fatalf("2=%+v, want c.txt file", resp.Entries[2])
	}
}

func TestFSMkdirAndDelete_CreatesAndRemoves(t *testing.T) {
	root := t.TempDir()
	a := &API{FSRoots: []FSRoot{{Name: "root", Path: root}}}

	// mkdir
	mkdirBody, _ := json.Marshal(map[string]any{"base": root, "path": "d1"})
	req := httptest.NewRequest(http.MethodPost, "/api/fs/mkdir", bytes.NewReader(mkdirBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mkdir status=%d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if info, err := os.Stat(filepath.Join(root, "d1")); err != nil || !info.IsDir() {
		t.Fatalf("expected directory created, err=%v", err)
	}

	// write a file then delete it
	if err := os.WriteFile(filepath.Join(root, "d1", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	delBody, _ := json.Marshal(map[string]any{"base": root, "path": filepath.Join("d1", "a.txt")})
	req = httptest.NewRequest(http.MethodPost, "/api/fs/delete", bytes.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status=%d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "d1", "a.txt")); err == nil {
		t.Fatalf("expected file to be deleted")
	}
}

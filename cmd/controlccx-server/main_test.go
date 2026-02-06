package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSpaOrFallback_ServesIndexForDeepLinkRoutes(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":      {Data: []byte("<html>INDEX</html>")},
		"assets/app.js":   {Data: []byte("console.log('ok')")},
		"favicon.svg":     {Data: []byte("<svg />")},
		"placeholder.txt": {Data: []byte("placeholder")},
	}
	h := spaOrFallback(fsys)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/skills", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (headers=%v body=%q)", rec.Code, rec.Header(), rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("expected no redirect Location header, got %q", loc)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("expected html Content-Type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "INDEX") {
		t.Fatalf("expected index.html body, got %q", rec.Body.String())
	}
}

func TestSpaOrFallback_ServesStaticAssetsWhenPresent(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<html>INDEX</html>")},
		"assets/app.js": {Data: []byte("console.log('ok')")},
	}
	h := spaOrFallback(fsys)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/assets/app.js", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (headers=%v body=%q)", rec.Code, rec.Header(), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "INDEX") {
		t.Fatalf("expected asset body, got %q", rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("expected no redirect Location header, got %q", loc)
	}
}

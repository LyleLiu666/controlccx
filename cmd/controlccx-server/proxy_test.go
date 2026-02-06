package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxySingleEndpoint_MapsForbiddenToSecretaryUnavailable(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	t.Cleanup(up.Close)

	h := proxySingleEndpoint(up.URL, "tok")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/chat", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "secretary_unavailable" {
		t.Fatalf("error=%v want %v", body["error"], "secretary_unavailable")
	}
}

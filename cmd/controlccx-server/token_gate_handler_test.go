package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"controlccx/internal/daemon"
)

func TestWithInstanceTokenGate_PassesThroughWhenLoopbackListen(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	h := withInstanceTokenGate("127.0.0.1:5174", "tok", next)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5174/api/system", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status=%d want %d", res.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatalf("expected next handler to be called")
	}
}

func TestWithInstanceTokenGate_RequiresTokenWhenNonLoopbackListen(t *testing.T) {
	nextCalls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	h := withInstanceTokenGate("0.0.0.0:5174", "tok", next)

	t.Run("missing", func(t *testing.T) {
		nextCalls = 0
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5174/api/system", nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want %d", res.Code, http.StatusUnauthorized)
		}
		var body struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Error != tokenGateErrorCode {
			t.Fatalf("error=%q want %q", body.Error, tokenGateErrorCode)
		}
		if nextCalls != 0 {
			t.Fatalf("nextCalls=%d want 0", nextCalls)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		nextCalls = 0
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5174/api/system", nil)
		req.Header.Set(daemon.InstanceTokenHeader, "bad")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want %d", res.Code, http.StatusUnauthorized)
		}
		if nextCalls != 0 {
			t.Fatalf("nextCalls=%d want 0", nextCalls)
		}
	})

	t.Run("valid", func(t *testing.T) {
		nextCalls = 0
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5174/api/system", nil)
		req.Header.Set(daemon.InstanceTokenHeader, "tok")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("status=%d want %d", res.Code, http.StatusNoContent)
		}
		if nextCalls != 1 {
			t.Fatalf("nextCalls=%d want 1", nextCalls)
		}
	})
}


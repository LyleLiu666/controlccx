package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenURLForListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{addr: "127.0.0.1:5174", want: "http://127.0.0.1:5174"},
		{addr: "0.0.0.0:5174", want: "http://127.0.0.1:5174"},
		{addr: ":5174", want: "http://127.0.0.1:5174"},
		{addr: "[::]:5174", want: "http://127.0.0.1:5174"},
		{addr: "localhost:5174", want: "http://localhost:5174"},
		{addr: "192.168.1.10:5174", want: "http://192.168.1.10:5174"},
	}
	for _, tc := range cases {
		got, err := openURLForListenAddr(tc.addr)
		if err != nil {
			t.Fatalf("addr=%q: unexpected error: %v", tc.addr, err)
		}
		if got != tc.want {
			t.Fatalf("addr=%q: got %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestBrowserOpenCommandForGOOS(t *testing.T) {
	url := "http://127.0.0.1:5174"

	name, args, err := browserOpenCommandForGOOS("darwin", url)
	if err != nil || name != "open" || len(args) != 1 || args[0] != url {
		t.Fatalf("darwin: name=%q args=%v err=%v", name, args, err)
	}

	name, args, err = browserOpenCommandForGOOS("windows", url)
	if err != nil || name != "rundll32" || len(args) != 2 || args[1] != url {
		t.Fatalf("windows: name=%q args=%v err=%v", name, args, err)
	}

	name, args, err = browserOpenCommandForGOOS("linux", url)
	if err != nil || name != "xdg-open" || len(args) != 1 || args[0] != url {
		t.Fatalf("linux: name=%q args=%v err=%v", name, args, err)
	}
}

func TestIsControlCCXRunning(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"claude":{"available":false},"codex":{"available":false}}`))
	}))
	defer okSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !isControlCCXRunning(ctx, okSrv.URL) {
		t.Fatalf("expected ok server to be detected as ControlCCX")
	}

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer badSrv.Close()

	if isControlCCXRunning(ctx, badSrv.URL) {
		t.Fatalf("expected bad server to not be detected as ControlCCX")
	}
}

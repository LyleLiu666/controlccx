package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSpeedTest_SucceedsOnHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	res := SpeedTest(context.Background(), srv.URL, SpeedTestOptions{Timeout: 200 * time.Millisecond})
	if !res.OK {
		t.Fatalf("expected ok, got=%+v", res)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status_code=%d", res.StatusCode)
	}
	if res.LatencyMS < 0 {
		t.Fatalf("latency_ms=%d", res.LatencyMS)
	}
}

func TestSpeedTest_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	res := SpeedTest(context.Background(), srv.URL, SpeedTestOptions{Timeout: 20 * time.Millisecond})
	if res.OK {
		t.Fatalf("expected not ok, got=%+v", res)
	}
	if res.Error == "" {
		t.Fatalf("expected error")
	}
	if res.Hint != "timeout" {
		t.Fatalf("hint=%q", res.Hint)
	}
}

func TestSpeedTest_InvalidURL(t *testing.T) {
	res := SpeedTest(context.Background(), " ", SpeedTestOptions{Timeout: 50 * time.Millisecond})
	if res.OK {
		t.Fatalf("expected not ok, got=%+v", res)
	}
	if res.Hint != "invalid_url" {
		t.Fatalf("hint=%q", res.Hint)
	}
}

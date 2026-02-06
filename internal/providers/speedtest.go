package providers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SpeedTestOptions struct {
	Timeout time.Duration
	Client  *http.Client // optional; useful for tests (e.g. TLS server)
}

type SpeedTestResult struct {
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
	Hint       string `json:"hint,omitempty"`
}

func SpeedTest(ctx context.Context, baseURL string, opts SpeedTestOptions) SpeedTestResult {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return SpeedTestResult{URL: baseURL, OK: false, Hint: "invalid_url", Error: "base url is required"}
	}
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return SpeedTestResult{URL: baseURL, OK: false, Hint: "invalid_url", Error: "invalid base url"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return SpeedTestResult{URL: baseURL, OK: false, Hint: "invalid_url", Error: "unsupported scheme"}
	}
	if strings.TrimSpace(u.Path) == "" {
		u.Path = "/"
	}
	targetURL := u.String()

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := opts.Client
	if client == nil {
		client = &http.Client{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return SpeedTestResult{URL: targetURL, OK: false, Hint: "invalid_url", Error: err.Error()}
	}

	start := time.Now()
	res, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return SpeedTestResult{
			URL:       targetURL,
			OK:        false,
			LatencyMS: latency.Milliseconds(),
			Error:     err.Error(),
			Hint:      hintFromSpeedTestError(err),
		}
	}
	defer func() { _ = res.Body.Close() }()
	return SpeedTestResult{
		URL:        targetURL,
		OK:         true,
		StatusCode: res.StatusCode,
		LatencyMS:  latency.Milliseconds(),
	}
}

func hintFromSpeedTestError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "no such host"):
		return "dns"
	case strings.Contains(msg, "connection refused"):
		return "refused"
	default:
		return "network_error"
	}
}

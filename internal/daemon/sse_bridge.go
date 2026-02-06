package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"controlccx/internal/events"
)

type SSEBridgeOptions struct {
	Client *http.Client
	Logf   func(format string, args ...any)
	Token  string
}

// BridgeSSEToHub connects to a remote SSE endpoint that emits JSON-encoded controlccx/internal/events.Event
// objects and republishes them into the provided hub. It will reconnect on failures until ctx is canceled.
func BridgeSSEToHub(ctx context.Context, sseURL string, dst *events.Hub, opts SSEBridgeOptions) error {
	if strings.TrimSpace(sseURL) == "" {
		return errors.New("sse bridge: url is required")
	}
	if dst == nil {
		return errors.New("sse bridge: destination hub is required")
	}

	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	logf := opts.Logf

	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "text/event-stream")
		if strings.TrimSpace(opts.Token) != "" {
			req.Header.Set(InstanceTokenHeader, strings.TrimSpace(opts.Token))
		}

		res, err := client.Do(req)
		if err != nil {
			if logf != nil && !isCancelErr(err) {
				logf("sse bridge connect error: %v", err)
			}
			if !sleepContext(ctx, backoff) {
				return nil
			}
			backoff = minDuration(backoff*2, 2*time.Second)
			continue
		}
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
			_ = res.Body.Close()
			msg := strings.TrimSpace(string(body))
			if msg == "" {
				msg = http.StatusText(res.StatusCode)
			}
			statusErr := fmt.Errorf("sse bridge: unexpected status %d: %s", res.StatusCode, msg)
			if logf != nil {
				logf("%v", statusErr)
			}
			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
				return statusErr
			}
			if !sleepContext(ctx, backoff) {
				return nil
			}
			backoff = minDuration(backoff*2, 2*time.Second)
			continue
		}

		backoff = 200 * time.Millisecond
		err = consumeSSE(res.Body, dst)
		_ = res.Body.Close()
		if err != nil && logf != nil && !isCancelErr(err) && !errors.Is(err, io.EOF) {
			logf("sse bridge stream error: %v", err)
		}

		if !sleepContext(ctx, backoff) {
			return nil
		}
		backoff = minDuration(backoff*2, 2*time.Second)
	}
}

func consumeSSE(r io.Reader, dst *events.Hub) error {
	br := bufio.NewReader(r)
	var (
		eventType string
		dataLines []string
	)

	flush := func() {
		if len(dataLines) == 0 {
			eventType = ""
			return
		}
		et := eventType
		raw := strings.TrimSpace(strings.Join(dataLines, "\n"))
		eventType = ""
		dataLines = nil
		if raw == "" {
			return
		}
		var evt events.Event
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			return
		}
		// Best-effort fallback: prefer the SSE event type when JSON type is missing.
		if strings.TrimSpace(evt.Type) == "" && strings.TrimSpace(et) != "" {
			evt.Type = strings.TrimSpace(et)
		}
		dst.Publish(evt)
	}

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				flush()
				return io.EOF
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		// Ignore other SSE fields.
	}
}

func isCancelErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	// Some read paths return non-wrapping errors with a cancellation string.
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "operation canceled")
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

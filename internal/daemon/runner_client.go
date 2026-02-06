package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type RunnerClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type RunnerClientOptions struct {
	Token  string
	Client *http.Client
}

func NewRunnerClient(baseURL string, opts RunnerClientOptions) (*RunnerClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("runner client: base url is required")
	}
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return nil, errors.New("runner client: instance token is required")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("runner client: invalid base url: %w", err)
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &RunnerClient{
		baseURL: baseURL,
		token:   token,
		client:  client,
	}, nil
}

func (c *RunnerClient) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

func (c *RunnerClient) Start(ctx context.Context, taskID string) error {
	if c == nil {
		return errors.New("runner client: nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("runner client: task id is required")
	}

	u := c.baseURL + path.Join("/api/runner/tasks/", taskID, "start")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(InstanceTokenHeader, c.token)
	res, err := c.client.Do(req)
	if err != nil {
		return &RunnerUnavailableError{Op: "start", BaseURL: c.baseURL, Err: err}
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return &RunnerResponseError{Op: "start", BaseURL: c.baseURL, StatusCode: res.StatusCode, Message: msg}
	}
	return nil
}

func (c *RunnerClient) Cancel(ctx context.Context, taskID string) (bool, error) {
	if c == nil {
		return false, errors.New("runner client: nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false, errors.New("runner client: task id is required")
	}
	u := c.baseURL + path.Join("/api/runner/tasks/", taskID, "cancel")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(InstanceTokenHeader, c.token)
	res, err := c.client.Do(req)
	if err != nil {
		return false, &RunnerUnavailableError{Op: "cancel", BaseURL: c.baseURL, Err: err}
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return false, &RunnerResponseError{Op: "cancel", BaseURL: c.baseURL, StatusCode: res.StatusCode, Message: msg}
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return false, err
	}
	return out.OK, nil
}

type RunnerUnavailableError struct {
	Op      string
	BaseURL string
	Err     error
}

func (e *RunnerUnavailableError) Error() string {
	op := strings.TrimSpace(e.Op)
	if op == "" {
		op = "request"
	}
	baseURL := strings.TrimSpace(e.BaseURL)
	if baseURL == "" {
		baseURL = "<unknown>"
	}
	if e.Err == nil {
		return fmt.Sprintf("runner %s unavailable at %s", op, baseURL)
	}
	return fmt.Sprintf("runner %s unavailable at %s: %v", op, baseURL, e.Err)
}

func (e *RunnerUnavailableError) Unwrap() error { return e.Err }

type RunnerResponseError struct {
	Op         string
	BaseURL    string
	StatusCode int
	Message    string
}

func (e *RunnerResponseError) Error() string {
	op := strings.TrimSpace(e.Op)
	if op == "" {
		op = "request"
	}
	baseURL := strings.TrimSpace(e.BaseURL)
	if baseURL == "" {
		baseURL = "<unknown>"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("runner %s failed at %s (status=%d): %s", op, baseURL, e.StatusCode, msg)
}

func IsRunnerUnavailable(err error) bool {
	var unavailable *RunnerUnavailableError
	if errors.As(err, &unavailable) {
		return true
	}
	return false
}

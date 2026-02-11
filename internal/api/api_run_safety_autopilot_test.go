package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestAPI_CreateTask_AppliesRunSafetyAutopilot(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "请总结这段代码在做什么",
		WorkDir:    ".",
	}
	buf, _ := json.Marshal(body)
	res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	bodyOut := decodeMutationResponse(t, res)
	requireMutationAction(t, bodyOut, "task.create")
	created := requireMutationTask(t, bodyOut)
	if strings.TrimSpace(created.TaskIntent) == "" || strings.TrimSpace(created.SafetyPreset) == "" {
		t.Fatalf("autopilot did not fill intent/preset: intent=%q preset=%q", created.TaskIntent, created.SafetyPreset)
	}
	if created.WorkerType != tasks.WorkerClaudeCode {
		t.Fatalf("worker_type=%q, want %q", created.WorkerType, tasks.WorkerClaudeCode)
	}
	if !created.ClaudeSandbox {
		t.Fatalf("claude_sandbox=%v, want true", created.ClaudeSandbox)
	}

	logs, err := taskStore.ListLogs(ctx, created.ID, 0, 200)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream == tasks.LogSystem && strings.Contains(l.Message, "safety.autopilot") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected system log containing safety.autopilot; logs=%v", logs)
	}
}

func TestAPI_CreateTask_RespectsExplicitRunSafetyFields(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body := tasks.CreateTaskInput{
		WorkerType:   tasks.WorkerClaudeCode,
		Mode:         tasks.ModeNew,
		Prompt:       "just run",
		WorkDir:      ".",
		TaskIntent:   "search-browse",
		SafetyPreset: "search-browse",
	}
	buf, _ := json.Marshal(body)
	res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	bodyOut := decodeMutationResponse(t, res)
	requireMutationAction(t, bodyOut, "task.create")
	created := requireMutationTask(t, bodyOut)
	if created.TaskIntent != "search-browse" || created.SafetyPreset != "search-browse" {
		t.Fatalf("unexpected intent/preset: intent=%q preset=%q", created.TaskIntent, created.SafetyPreset)
	}
}

func TestAPI_ResumeTask_SafetyEnvelopeDoesNotOverridePreviousSafety(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	createBody := tasks.CreateTaskInput{
		WorkerType:          tasks.WorkerCodex,
		Mode:                tasks.ModeNew,
		Prompt:              "search-browse: find docs for xyz",
		WorkDir:             ".",
		SessionID:           "sess-123",
		TaskIntent:          "search-browse",
		SafetyPreset:        "search-browse",
		CodexSandbox:        "workspace-write",
		CodexApprovalPolicy: "never",
		CodexSearch:         true,
	}
	createBuf, _ := json.Marshal(createBody)
	createRes, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(createBuf))
	if err != nil {
		t.Fatalf("post create: %v", err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d, want 200", createRes.StatusCode)
	}

	createOut := decodeMutationResponse(t, createRes)
	requireMutationAction(t, createOut, "task.create")
	created := requireMutationTask(t, createOut)
	exitCode := 0
	if err := taskStore.FinishTask(ctx, created.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  created.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish created: %v", err)
	}

	resumePayload := map[string]any{
		"prompt":          "continue",
		"safety_envelope": "install-enabled",
	}
	resumeBuf, _ := json.Marshal(resumePayload)
	resumeRes, err := http.Post(srv.URL+"/api/tasks/"+created.ID+"/resume", "application/json", bytes.NewReader(resumeBuf))
	if err != nil {
		t.Fatalf("post resume: %v", err)
	}
	defer resumeRes.Body.Close()
	if resumeRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resumeRes.Body)
		t.Fatalf("resume status=%d, want 200; body=%s", resumeRes.StatusCode, strings.TrimSpace(string(b)))
	}

	resumeOut := decodeMutationResponse(t, resumeRes)
	requireMutationAction(t, resumeOut, "task.resume")
	resumed := requireMutationTask(t, resumeOut)

	if resumed.Mode != tasks.ModeResume {
		t.Fatalf("resumed mode=%q, want %q", resumed.Mode, tasks.ModeResume)
	}
	if resumed.TaskIntent != created.TaskIntent || resumed.SafetyPreset != created.SafetyPreset {
		t.Fatalf("resumed intent/preset changed: intent=%q preset=%q (want intent=%q preset=%q)",
			resumed.TaskIntent, resumed.SafetyPreset, created.TaskIntent, created.SafetyPreset)
	}
	if resumed.CodexSandbox != created.CodexSandbox || resumed.CodexApprovalPolicy != created.CodexApprovalPolicy || resumed.CodexSearch != created.CodexSearch {
		t.Fatalf("resumed codex settings changed: sandbox=%q approval=%q search=%v (want sandbox=%q approval=%q search=%v)",
			resumed.CodexSandbox, resumed.CodexApprovalPolicy, resumed.CodexSearch,
			created.CodexSandbox, created.CodexApprovalPolicy, created.CodexSearch)
	}
}

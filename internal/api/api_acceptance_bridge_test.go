package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_Acceptance_IncludesMissionContractBridgedCriteria(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    filepath.Join(t.TempDir(), "proj"),
		SessionID:  "sess-acc-bridge",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	key := tasks.SessionKeyForTask(task)
	if _, err := taskStore.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  key,
		Goal: "ship safely",
		AcceptanceCriteria: []string{
			"go test ./... passes",
			"文档可读性高，适合团队接手",
		},
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := taskStore.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
		Key:       key,
		Status:    "running",
		Iteration: 1,
		Summary:   "bridging",
	}); err != nil {
		t.Fatalf("upsert acceptance state: %v", err)
	}

	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/acceptance?key=" + url.QueryEscape(key))
	if err != nil {
		t.Fatalf("get acceptance: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var out struct {
		OK    bool                   `json:"ok"`
		State *tasks.AcceptanceState `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.OK || out.State == nil {
		t.Fatalf("unexpected response: %+v", out)
	}
	if out.State.PlanJSON == "" {
		t.Fatalf("plan_json empty, want bridged criteria")
	}

	var plan struct {
		Criteria []struct {
			CriterionID string `json:"criterion_id"`
		} `json:"criteria"`
	}
	if err := json.Unmarshal([]byte(out.State.PlanJSON), &plan); err != nil {
		t.Fatalf("decode plan_json: %v", err)
	}
	if len(plan.Criteria) < 2 {
		t.Fatalf("criteria len=%d, want >=2", len(plan.Criteria))
	}
	if plan.Criteria[0].CriterionID != "ac-001" {
		t.Fatalf("criteria[0].criterion_id=%q, want %q", plan.Criteria[0].CriterionID, "ac-001")
	}
}

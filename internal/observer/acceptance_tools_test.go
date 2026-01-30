package observer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestTools_acceptancePrepare_CreatesAndAdvancesIterationByRunID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "build a project",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	res1, err := tools["acceptance_prepare"].Run(ctx, map[string]any{
		"task_id":        first.ID,
		"max_iterations": 10,
	})
	if err != nil {
		t.Fatalf("acceptance_prepare first: %v", err)
	}
	m1 := res1.(map[string]any)
	if m1["can_continue"] != true {
		t.Fatalf("can_continue=%v want true", m1["can_continue"])
	}
	state1 := m1["state"].(tasks.AcceptanceState)
	if state1.Iteration != 1 || state1.RunID != first.ID {
		t.Fatalf("state1=%+v", state1)
	}

	// Same run id should NOT advance.
	res1b, err := tools["acceptance_prepare"].Run(ctx, map[string]any{
		"task_id":        first.ID,
		"max_iterations": 10,
	})
	if err != nil {
		t.Fatalf("acceptance_prepare first again: %v", err)
	}
	state1b := res1b.(map[string]any)["state"].(tasks.AcceptanceState)
	if state1b.Iteration != 1 || state1b.RunID != first.ID {
		t.Fatalf("state1b=%+v", state1b)
	}

	// New run id in same session advances iteration.
	res2, err := tools["acceptance_prepare"].Run(ctx, map[string]any{
		"task_id":        second.ID,
		"max_iterations": 10,
	})
	if err != nil {
		t.Fatalf("acceptance_prepare second: %v", err)
	}
	state2 := res2.(map[string]any)["state"].(tasks.AcceptanceState)
	if state2.Iteration != 2 || state2.RunID != second.ID {
		t.Fatalf("state2=%+v", state2)
	}
}

func TestTools_acceptancePrepare_StopsAtIterationLimit(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "write >=800字",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	// Seed acceptance state as already at max.
	if _, err := store.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
		Key:           "s:sess-1",
		Status:        "running",
		Iteration:     10,
		MaxIterations: 10,
		CurrentGate:   "objective",
		Summary:       "still failing",
		PlanJSON:      "{}",
		Report:        "",
		RunID:         first.ID,
	}); err != nil {
		t.Fatalf("seed acceptance: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	res, err := tools["acceptance_prepare"].Run(ctx, map[string]any{
		"task_id":        second.ID,
		"max_iterations": 10,
	})
	if err != nil {
		t.Fatalf("acceptance_prepare: %v", err)
	}
	m := res.(map[string]any)
	if m["can_continue"] != false {
		t.Fatalf("can_continue=%v want false", m["can_continue"])
	}
	state := m["state"].(tasks.AcceptanceState)
	if state.Iteration != 10 || strings.ToLower(state.Status) != "failed" {
		t.Fatalf("state=%+v", state)
	}
}

func TestTools_acceptanceClassify_UsesDeterministicHeuristics(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	simple, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "把这段话翻译成英文",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create simple: %v", err)
	}
	complex, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "从0开发一个前后端+数据库的可运行项目，并写README和测试",
		WorkDir:    ".",
		SessionID:  "sess-2",
	})
	if err != nil {
		t.Fatalf("create complex: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	r1, err := tools["acceptance_classify"].Run(ctx, map[string]any{"task_id": simple.ID})
	if err != nil {
		t.Fatalf("classify simple: %v", err)
	}
	if r1.(map[string]any)["complex"] != false {
		t.Fatalf("simple classified as complex: %v", r1)
	}

	r2, err := tools["acceptance_classify"].Run(ctx, map[string]any{"task_id": complex.ID})
	if err != nil {
		t.Fatalf("classify complex: %v", err)
	}
	if r2.(map[string]any)["complex"] != true {
		t.Fatalf("complex classified as simple: %v", r2)
	}
}

func TestTools_acceptanceBuildContract_ExtractsObjectiveAndRubricHints(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一篇不少于30000字、包含14个部分、适合公众号的文章",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	res, err := tools["acceptance_build_contract"].Run(ctx, map[string]any{
		"task_id": task.ID,
	})
	if err != nil {
		t.Fatalf("acceptance_build_contract: %v", err)
	}
	m := res.(map[string]any)
	planJSON := m["plan_json"].(string)

	var plan AcceptancePlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		t.Fatalf("unmarshal plan_json: %v", err)
	}
	if len(plan.ObjectiveCriteria) < 2 {
		t.Fatalf("objective_criteria=%v", plan.ObjectiveCriteria)
	}
	if plan.ObjectiveCriteria[0].Min <= 0 && plan.ObjectiveCriteria[1].Min <= 0 {
		t.Fatalf("expected min constraints, got %v", plan.ObjectiveCriteria)
	}
	if len(plan.SubjectiveRubrics) == 0 {
		t.Fatalf("expected subjective rubrics for wechat, got none")
	}
}

func TestTools_acceptanceEvaluateObjectives_EvaluatesAgainstOutput(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "write >=3 sections",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, err = store.AppendLog(ctx, task.ID, tasks.LogAssistant, strings.Join([]string{
		"# Title",
		"## A",
		"## B",
		"body",
	}, "\n"))
	if err != nil {
		t.Fatalf("append log: %v", err)
	}
	if err := store.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	plan := AcceptancePlan{
		IntentSummary: "article",
		ObjectiveCriteria: []AcceptanceObjectiveCriterion{
			{ID: "sections", Title: ">=3 sections", Method: "task_output_stats.sections", Min: 3},
		},
	}
	planBytes, _ := json.Marshal(plan)
	res, err := tools["acceptance_evaluate_objectives"].Run(ctx, map[string]any{
		"task_id":   task.ID,
		"plan_json": string(planBytes),
	})
	if err != nil {
		t.Fatalf("acceptance_evaluate_objectives: %v", err)
	}
	m := res.(map[string]any)
	results := m["results"].([]AcceptanceObjectiveResult)
	if len(results) != 1 {
		t.Fatalf("results=%v", results)
	}
	if !results[0].Pass {
		t.Fatalf("expected pass, got %v", results[0])
	}
}

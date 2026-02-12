package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

type queueTestRunner struct {
	startCalls  []string
	cancelCalls []string
	cancelOK    bool
}

func (r *queueTestRunner) Start(ctx context.Context, taskID string) error {
	r.startCalls = append(r.startCalls, taskID)
	return nil
}

func (r *queueTestRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	r.cancelCalls = append(r.cancelCalls, taskID)
	return r.cancelOK, nil
}

func TestAPI_SessionPreemptContinue_QueuesAheadOfNormalContinue(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	runner := &queueTestRunner{cancelOK: true}
	apiSvc := &API{Tasks: taskStore, Hub: hub, Workers: runner}

	a, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := taskStore.SetRunning(ctx, a.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(a)

	bPayload, _ := json.Marshal(map[string]any{"prompt": "B"})
	bRes, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(bPayload))
	if err != nil {
		t.Fatalf("queue B: %v", err)
	}
	t.Cleanup(func() { _ = bRes.Body.Close() })
	if bRes.StatusCode != http.StatusAccepted {
		t.Fatalf("continue status=%d, want 202", bRes.StatusCode)
	}

	cPayload, _ := json.Marshal(map[string]any{"prompt": "C"})
	cRes, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/preempt-continue", "application/json", bytes.NewReader(cPayload))
	if err != nil {
		t.Fatalf("preempt C: %v", err)
	}
	t.Cleanup(func() { _ = cRes.Body.Close() })
	if cRes.StatusCode != http.StatusAccepted {
		t.Fatalf("preempt status=%d, want 202", cRes.StatusCode)
	}
	bodyOut := decodeMutationResponse(t, cRes)
	requireMutationAction(t, bodyOut, "session.preempt_continue")
	cAck := requireMutationQueue(t, bodyOut)
	if ok, _ := cAck["queued"].(bool); !ok {
		t.Fatalf("queued=%v, want true", cAck["queued"])
	}
	if got := anyString(cAck["preempted_task_id"]); got != a.ID {
		t.Fatalf("preempted_task_id=%q, want %q", got, a.ID)
	}
	if len(runner.cancelCalls) != 1 || runner.cancelCalls[0] != a.ID {
		t.Fatalf("cancel calls=%v, want [%s]", runner.cancelCalls, a.ID)
	}

	qRes, err := http.Get(srv.URL + "/api/sessions/" + url.PathEscape(key) + "/queue")
	if err != nil {
		t.Fatalf("queue list: %v", err)
	}
	t.Cleanup(func() { _ = qRes.Body.Close() })
	if qRes.StatusCode != http.StatusOK {
		t.Fatalf("queue list status=%d, want 200", qRes.StatusCode)
	}
	var q struct {
		Items []tasks.SessionContinueQueueItem `json:"items"`
	}
	if err := json.NewDecoder(qRes.Body).Decode(&q); err != nil {
		t.Fatalf("decode queue list: %v", err)
	}
	if len(q.Items) != 2 {
		t.Fatalf("queue items=%d, want 2", len(q.Items))
	}
	if q.Items[0].Prompt != "C" {
		t.Fatalf("queue[0].prompt=%q, want %q", q.Items[0].Prompt, "C")
	}
	if q.Items[1].Prompt != "B" {
		t.Fatalf("queue[1].prompt=%q, want %q", q.Items[1].Prompt, "B")
	}
}

func TestAPI_DrainContinueQueue_PreservesPreemptOrderCBeforeB(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	a, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := taskStore.SetRunning(ctx, a.ID); err != nil {
		t.Fatalf("set running A: %v", err)
	}

	optsB, _ := json.Marshal(sessionContinueOptions{Prompt: "B"})
	optsC, _ := json.Marshal(sessionContinueOptions{Prompt: "C"})
	if _, err := taskStore.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: a.ConversationID,
		Prompt:         "B",
		RunOptionsJSON: string(optsB),
		Priority:       0,
		Source:         "continue",
	}); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	if _, err := taskStore.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: a.ConversationID,
		Prompt:         "C",
		RunOptionsJSON: string(optsC),
		Priority:       100,
		Source:         "preempt",
	}); err != nil {
		t.Fatalf("enqueue C: %v", err)
	}

	if err := taskStore.FinishTask(ctx, a.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusCanceled,
		SessionID:  a.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish A canceled: %v", err)
	}

	if err := apiSvc.drainContinueQueuesOnce(ctx); err != nil {
		t.Fatalf("drain #1: %v", err)
	}
	runs, err := taskStore.ListTasksByConversationID(ctx, a.ConversationID, 10, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list runs #1: %v", err)
	}
	var cRun tasks.Task
	foundC := false
	for _, r := range runs {
		if r.ID == a.ID {
			continue
		}
		if r.Prompt == "C" {
			cRun = r
			foundC = true
			break
		}
	}
	if !foundC {
		t.Fatalf("expected first drained run prompt C")
	}

	if err := taskStore.FinishTask(ctx, cRun.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  cRun.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish C: %v", err)
	}
	if err := apiSvc.drainContinueQueuesOnce(ctx); err != nil {
		t.Fatalf("drain #2: %v", err)
	}

	runs2, err := taskStore.ListTasksByConversationID(ctx, a.ConversationID, 20, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list runs #2: %v", err)
	}
	var hasB bool
	for _, r := range runs2 {
		if r.Prompt == "B" {
			hasB = true
			break
		}
	}
	if !hasB {
		t.Fatalf("expected B to be created after C finished")
	}
}

func TestAPI_DrainContinueQueue_EnforcesProjectBudgetAndFairness(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	apiSvc := &API{Tasks: taskStore, Hub: events.NewHub()}
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")

	// Project A has one in-flight run already.
	aRun, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerClaudeCode,
		Mode:            tasks.ModeNew,
		Prompt:          "A-running",
		WorkDir:         filepath.Join(repoA, ".ccx", "worktrees", "run"),
		ConversationID:  "conv-a-run",
		WorkDirStrategy: "worktree",
		BaseWorkDir:     repoA,
		WorktreeDir:     filepath.Join(repoA, ".ccx", "worktrees", "run"),
		WorktreeBranch:  "ccx-a-run",
	})
	if err != nil {
		t.Fatalf("create A running: %v", err)
	}
	if err := taskStore.SetRunning(ctx, aRun.ID); err != nil {
		t.Fatalf("set A running: %v", err)
	}

	// Project A backlog conversation (same base project scope as A running).
	aSeed, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerClaudeCode,
		Mode:            tasks.ModeNew,
		Prompt:          "A-seed",
		WorkDir:         filepath.Join(repoA, ".ccx", "worktrees", "a-backlog"),
		SessionID:       "sess-a-backlog",
		ConversationID:  "conv-a-backlog",
		WorkDirStrategy: "worktree",
		BaseWorkDir:     repoA,
		WorktreeDir:     filepath.Join(repoA, ".ccx", "worktrees", "a-backlog"),
		WorktreeBranch:  "ccx-a-backlog",
	})
	if err != nil {
		t.Fatalf("create A backlog seed: %v", err)
	}
	if err := taskStore.FinishTask(ctx, aSeed.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  aSeed.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish A backlog seed: %v", err)
	}

	// Project B backlog conversation.
	bSeed, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerClaudeCode,
		Mode:            tasks.ModeNew,
		Prompt:          "B-seed",
		WorkDir:         filepath.Join(repoB, ".ccx", "worktrees", "b-backlog"),
		SessionID:       "sess-b-backlog",
		ConversationID:  "conv-b-backlog",
		WorkDirStrategy: "worktree",
		BaseWorkDir:     repoB,
		WorktreeDir:     filepath.Join(repoB, ".ccx", "worktrees", "b-backlog"),
		WorktreeBranch:  "ccx-b-backlog",
	})
	if err != nil {
		t.Fatalf("create B backlog seed: %v", err)
	}
	if err := taskStore.FinishTask(ctx, bSeed.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  bSeed.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish B backlog seed: %v", err)
	}

	optsA, _ := json.Marshal(sessionContinueOptions{Prompt: "A-next"})
	itemA, err := taskStore.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: aSeed.ConversationID,
		Prompt:         "A-next",
		RunOptionsJSON: string(optsA),
		Priority:       0,
		Source:         "continue",
	})
	if err != nil {
		t.Fatalf("enqueue A: %v", err)
	}
	optsB, _ := json.Marshal(sessionContinueOptions{Prompt: "B-next"})
	itemB, err := taskStore.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: bSeed.ConversationID,
		Prompt:         "B-next",
		RunOptionsJSON: string(optsB),
		Priority:       0,
		Source:         "continue",
	})
	if err != nil {
		t.Fatalf("enqueue B: %v", err)
	}

	if err := apiSvc.drainContinueQueuesOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	gotA, err := taskStore.GetSessionContinueQueueItem(ctx, itemA.ID)
	if err != nil {
		t.Fatalf("get A item: %v", err)
	}
	gotB, err := taskStore.GetSessionContinueQueueItem(ctx, itemB.ID)
	if err != nil {
		t.Fatalf("get B item: %v", err)
	}

	// Project A should be held by budget (already has one in-flight run),
	// while project B should still make progress.
	if gotA.State != tasks.SessionContinueQueueStatePending {
		t.Fatalf("A state=%q, want %q", gotA.State, tasks.SessionContinueQueueStatePending)
	}
	if gotB.State != tasks.SessionContinueQueueStateDone {
		t.Fatalf("B state=%q, want %q", gotB.State, tasks.SessionContinueQueueStateDone)
	}
}

func TestAPI_SessionContinue_CrossSessionSameWorkdirStillBlocked(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	runA, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A1",
		WorkDir:    ".",
		SessionID:  "sess-a",
	})
	if err != nil {
		t.Fatalf("create runA: %v", err)
	}
	if err := taskStore.SetRunning(ctx, runA.ID); err != nil {
		t.Fatalf("set runA running: %v", err)
	}

	runB, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "B1",
		WorkDir:    ".",
		SessionID:  "sess-b",
	})
	if err == nil {
		t.Fatalf("expected create runB to fail while runA in same workdir is in-flight")
	}

	if err := taskStore.FinishTask(ctx, runA.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  runA.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish runA: %v", err)
	}

	// Create runB after A completed so session B exists.
	runB, err = taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "B1",
		WorkDir:    ".",
		SessionID:  "sess-b",
	})
	if err != nil {
		t.Fatalf("create runB after A finished: %v", err)
	}
	if err := taskStore.FinishTask(ctx, runB.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  runB.SessionID,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish runB: %v", err)
	}

	// Keep session A in-flight again.
	runA2, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "A2",
		WorkDir:    ".",
		SessionID:  "sess-a",
	})
	if err != nil {
		t.Fatalf("create runA2: %v", err)
	}
	if err := taskStore.SetRunning(ctx, runA2.ID); err != nil {
		t.Fatalf("set runA2 running: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)
	keyB := tasks.SessionKeyForTask(runB)
	payload, _ := json.Marshal(map[string]any{"prompt": "continue B"})
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(keyB)+"/continue", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("continue session B: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409 workdir_busy", res.StatusCode)
	}
}

func TestAPI_DrainContinueQueue_BlockedSessionKeepsPending(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	run, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A1",
		WorkDir:    ".",
		SessionID:  "sess-a",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := taskStore.SetBlocked(ctx, run.ID); err != nil {
		t.Fatalf("set blocked: %v", err)
	}

	opts, _ := json.Marshal(sessionContinueOptions{Prompt: "continue"})
	item, err := taskStore.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: run.ConversationID,
		Prompt:         "continue",
		RunOptionsJSON: string(opts),
		Priority:       0,
		Source:         "continue",
	})
	if err != nil {
		t.Fatalf("enqueue continue: %v", err)
	}

	if err := apiSvc.drainContinueQueuesOnce(ctx); err != nil {
		t.Fatalf("drain queue: %v", err)
	}

	got, err := taskStore.GetSessionContinueQueueItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("get queue item: %v", err)
	}
	if got.State != tasks.SessionContinueQueueStatePending {
		t.Fatalf("queue state=%q, want %q", got.State, tasks.SessionContinueQueueStatePending)
	}
}

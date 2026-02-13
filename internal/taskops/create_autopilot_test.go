package taskops

import (
	"context"
	"testing"

	"controlccx/internal/tasks"
)

type createRecordingRunner struct {
	started []string
}

func (r *createRecordingRunner) Start(ctx context.Context, taskID string) error {
	_ = ctx
	r.started = append(r.started, taskID)
	return nil
}

func (r *createRecordingRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	_ = ctx
	_ = taskID
	return false, nil
}

type fixedLLM struct {
	out   string
	calls int
}

func (f *fixedLLM) Name() string { return "fixed-llm" }

func (f *fixedLLM) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	f.calls++
	return f.out, nil
}

func TestService_CreateTask_DoesNotStartWaitingTasks(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	runner := &createRecordingRunner{}
	svc.Workers = runner

	if _, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo A",
		WorkDir:    ".",
	}); err != nil {
		t.Fatalf("create first: %v", err)
	}

	task, err := svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo B",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Status != tasks.StatusWaiting {
		t.Fatalf("status=%q, want %q", task.Status, tasks.StatusWaiting)
	}
	if len(runner.started) != 0 {
		t.Fatalf("started=%v, want none (waiting tasks should not be started immediately)", runner.started)
	}
}

func TestService_CreateTask_AutopilotUsesLLMWhenConfigured(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	llm := &fixedLLM{out: `{"intent":"analyze","confidence":0.9,"signals":["llm"],"reason":"summarize request"}`}
	svc.AutopilotLLM = llm

	task, err := svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "Summarize the repository structure and responsibilities.",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.TaskIntent != "analyze" {
		t.Fatalf("task_intent=%q, want %q", task.TaskIntent, "analyze")
	}
	if llm.calls != 1 {
		t.Fatalf("llm calls=%d, want 1", llm.calls)
	}
}

func TestService_CreateTask_WorkdirBusy_ReadOnlyIntentDefaultsToWait(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	runner := &createRecordingRunner{}
	svc.Workers = runner

	llm := &fixedLLM{out: `{"intent":"analyze","confidence":0.9,"signals":["llm"],"reason":"summarize request"}`}
	svc.AutopilotLLM = llm

	if _, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo A",
		WorkDir:    ".",
	}); err != nil {
		t.Fatalf("create first: %v", err)
	}

	task, err := svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "Summarize the repository structure and responsibilities.",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Status != tasks.StatusWaiting {
		t.Fatalf("status=%q, want %q", task.Status, tasks.StatusWaiting)
	}
	if task.WorkDirStrategy != "wait" {
		t.Fatalf("workdir_strategy=%q, want %q", task.WorkDirStrategy, "wait")
	}
	if len(runner.started) != 0 {
		t.Fatalf("started=%v, want none (waiting tasks should not be started immediately)", runner.started)
	}
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/db"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

func newDepsForToolsTest(t *testing.T) (context.Context, Deps) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := tasks.NewStore(conn)
	ops := &taskops.Service{Tasks: store}
	return ctx, Deps{Tasks: store, Ops: ops}
}

func TestDescriptors_UniqueNonEmpty(t *testing.T) {
	ds := Descriptors()
	if len(ds) == 0 {
		t.Fatalf("expected descriptors")
	}
	seen := map[string]bool{}
	for _, d := range ds {
		if strings.TrimSpace(d.Name) == "" {
			t.Fatalf("empty tool name")
		}
		if seen[d.Name] {
			t.Fatalf("duplicate tool name: %s", d.Name)
		}
		seen[d.Name] = true
		if strings.TrimSpace(d.DescriptionZH) == "" {
			t.Fatalf("empty description for %s", d.Name)
		}
	}
}

func TestTaskLogsTail_BoundedAndTruncated(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	suffix := "Operation not permitted"
	for i := 0; i < 30; i++ {
		// Ensure we exceed the 800 rune cap so head+tail truncation is exercised.
		msg := strings.Repeat("中", 1500) + " " + suffix
		if _, err := deps.Tasks.AppendLog(ctx, task.ID, tasks.LogSystem, msg); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	reg := NewRegistry(deps)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_logs_tail",
		Fields: map[string]string{
			"task_id": task.ID,
			"count":   "100",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	var out struct {
		Count int `json:"count"`
		Logs  []struct {
			Message   string `json:"message"`
			Truncated bool   `json:"truncated"`
		} `json:"logs"`
	}
	raw, err := json.Marshal(outAny)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v raw=%s", err, string(raw))
	}
	if out.Count != 20 {
		t.Fatalf("count=%d want 20", out.Count)
	}
	if len(out.Logs) != 20 {
		t.Fatalf("logs len=%d want 20", len(out.Logs))
	}
	for _, l := range out.Logs {
		msg := strings.TrimSpace(l.Message)
		if len([]rune(msg)) > 800 {
			t.Fatalf("line too long: %d", len([]rune(msg)))
		}
		if !l.Truncated {
			t.Fatalf("expected truncated=true, got false (msg=%q)", msg)
		}
		if !strings.Contains(msg, suffix) {
			t.Fatalf("expected tail to retain suffix %q, got %q", suffix, msg)
		}
	}
}

func TestTaskLogsTail_UsesLatestLogsWhenManyEntries(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for i := 1; i <= 2105; i++ {
		if _, err := deps.Tasks.AppendLog(ctx, task.ID, tasks.LogSystem, fmt.Sprintf("line-%04d", i)); err != nil {
			t.Fatalf("append log %d: %v", i, err)
		}
	}

	reg := NewRegistry(deps)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_logs_tail",
		Fields: map[string]string{
			"task_id": task.ID,
			"count":   "20",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	var out struct {
		Count int `json:"count"`
		Logs  []struct {
			Message string `json:"message"`
		} `json:"logs"`
	}
	raw, err := json.Marshal(outAny)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v raw=%s", err, string(raw))
	}
	if out.Count != 20 || len(out.Logs) != 20 {
		t.Fatalf("count/logs mismatch: count=%d len=%d", out.Count, len(out.Logs))
	}
	if out.Logs[0].Message != "line-2086" {
		t.Fatalf("first tail log=%q want %q", out.Logs[0].Message, "line-2086")
	}
	if out.Logs[19].Message != "line-2105" {
		t.Fatalf("last tail log=%q want %q", out.Logs[19].Message, "line-2105")
	}
}

func TestTasksList_ProjectScopeGuardByTaskID(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	projA := filepath.Join(t.TempDir(), "proj-a")
	projB := filepath.Join(t.TempDir(), "proj-b")
	taskA, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerExec,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-scope-a",
		Prompt:         "a",
		WorkDir:        projA,
	})
	if err != nil {
		t.Fatalf("create task A: %v", err)
	}
	if _, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerExec,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-scope-b",
		Prompt:         "b",
		WorkDir:        projB,
	}); err != nil {
		t.Fatalf("create task B: %v", err)
	}

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "tasks_list",
		Fields: map[string]string{
			"task_id":         taskA.ID,
			"limit":           "50",
			"include_deleted": "1",
		},
	})
	if err != nil {
		t.Fatalf("tasks_list by task_id: %v", err)
	}
	var out struct {
		ProjectScope string `json:"project_scope"`
		Tasks        []struct {
			ID      string `json:"id"`
			WorkDir string `json:"workdir"`
		} `json:"tasks"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	wantScope := tasks.NormalizeProjectKey(projA)
	if out.ProjectScope != wantScope {
		t.Fatalf("project_scope=%q, want %q", out.ProjectScope, wantScope)
	}
	if len(out.Tasks) == 0 {
		t.Fatalf("expected scoped tasks")
	}
	for _, item := range out.Tasks {
		if tasks.NormalizeProjectKey(item.WorkDir) != wantScope {
			t.Fatalf("unexpected cross-project workdir=%q want scope %q", item.WorkDir, wantScope)
		}
	}
}

func TestTasksList_ProjectScopeGuardByConversationID(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	projA := filepath.Join(t.TempDir(), "proj-a")
	projB := filepath.Join(t.TempDir(), "proj-b")
	if _, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerExec,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-scope2-a",
		Prompt:         "a",
		WorkDir:        projA,
	}); err != nil {
		t.Fatalf("create task A: %v", err)
	}
	if _, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerExec,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-scope2-b",
		Prompt:         "b",
		WorkDir:        projB,
	}); err != nil {
		t.Fatalf("create task B: %v", err)
	}

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "tasks_list",
		Fields: map[string]string{
			"conversation_id": "conv-scope2-a",
			"limit":           "50",
			"include_deleted": "1",
		},
	})
	if err != nil {
		t.Fatalf("tasks_list by conversation_id: %v", err)
	}
	var out struct {
		ProjectScope string `json:"project_scope"`
		Tasks        []struct {
			WorkDir string `json:"workdir"`
		} `json:"tasks"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	wantScope := tasks.NormalizeProjectKey(projA)
	if out.ProjectScope != wantScope {
		t.Fatalf("project_scope=%q, want %q", out.ProjectScope, wantScope)
	}
	if len(out.Tasks) == 0 {
		t.Fatalf("expected scoped tasks")
	}
	for _, item := range out.Tasks {
		if tasks.NormalizeProjectKey(item.WorkDir) != wantScope {
			t.Fatalf("unexpected cross-project workdir=%q want scope %q", item.WorkDir, wantScope)
		}
	}
}

func TestTaskLogGet_CapsAt12000(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	suffix := "Operation not permitted"
	entry, err := deps.Tasks.AppendLog(ctx, task.ID, tasks.LogSystem, strings.Repeat("A", 15000)+"\n"+suffix)
	if err != nil {
		t.Fatalf("append log: %v", err)
	}

	reg := NewRegistry(deps)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_log_get",
		Fields: map[string]string{
			"task_id": task.ID,
			"log_id":  "" + strings.TrimSpace(""),
		},
	})
	if err == nil {
		t.Fatalf("expected log_id required error")
	}
	outAny, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_log_get",
		Fields: map[string]string{
			"task_id": task.ID,
			"log_id":  strconvFormatInt(entry.ID),
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	out := outAny.(map[string]any)
	msg := out["message"].(string)
	if len([]rune(msg)) != 12000 {
		t.Fatalf("message len=%d want 12000", len([]rune(msg)))
	}
	if out["truncated"] != true {
		t.Fatalf("truncated=%v want true", out["truncated"])
	}
	if !strings.Contains(msg, suffix) {
		t.Fatalf("expected tail to retain suffix %q, got %q", suffix, msg)
	}
}

func TestTaskContinueSubmit_RejectsCancelPrompt(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	reg := NewRegistry(deps)
	for _, prompt := range []string{"/cancel", " /Cancel now ", "cancel", "CANCEL!", "取消", "取消一下"} {
		_, err = reg.Execute(ctx, agentsdk.ToolCall{
			Name: "task_continue_submit",
			Fields: map[string]string{
				"task_id": task.ID,
				"prompt":  prompt,
			},
		})
		if err == nil {
			t.Fatalf("expected error for prompt=%q", prompt)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "task_cancel_submit") {
			t.Fatalf("err=%q want mention task_cancel_submit (prompt=%q)", err.Error(), prompt)
		}
	}
}

func TestTaskCancelSubmit_CancelsQueued(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	reg := NewRegistry(deps)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_cancel_submit",
		Fields: map[string]string{
			"task_id": task.ID,
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	out := outAny.(map[string]any)
	if out["requested"] != true {
		t.Fatalf("requested=%v want true", out["requested"])
	}
	if strings.TrimSpace(out["status_before"].(string)) != string(tasks.StatusQueued) {
		t.Fatalf("status_before=%v want %v", out["status_before"], tasks.StatusQueued)
	}
	if strings.TrimSpace(out["status_after"].(string)) != string(tasks.StatusCanceled) {
		t.Fatalf("status_after=%v want %v", out["status_after"], tasks.StatusCanceled)
	}

	updated, err := deps.Tasks.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusCanceled {
		t.Fatalf("status=%q want %q", updated.Status, tasks.StatusCanceled)
	}
}

func TestTaskApprovalDecide_AutoResolvePending(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ar, err := deps.Tasks.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "s",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	reg := NewRegistry(deps)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_approval_decide",
		Fields: map[string]string{
			"task_id":  task.ID,
			"decision": "approve",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	out := outAny.(map[string]any)
	if strings.TrimSpace(fmt.Sprint(out["approval_id"])) != ar.ID {
		t.Fatalf("approval_id=%v want %s", out["approval_id"], ar.ID)
	}
	if strings.TrimSpace(fmt.Sprint(out["status"])) != string(tasks.ApprovalStatusApproved) {
		t.Fatalf("status=%v", out["status"])
	}
}

func TestTaskEnterUnsafe_RequiresConfirm(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	reg := NewRegistry(deps)
	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_enter_unsafe_submit",
		Fields: map[string]string{
			"task_id": task.ID,
		},
	})
	if err == nil {
		t.Fatalf("expected confirm error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "confirm=true") {
		t.Fatalf("unexpected error: %v", err)
	}
	logs, err := deps.Tasks.ListLogs(ctx, task.ID, 0, 200)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream == tasks.LogSystem && strings.Contains(l.Message, "task_enter_unsafe_submit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audit log after denied unsafe action")
	}
}

func TestTaskNewSubmit_RequiresExplicitFields(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)
	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_new_submit",
		Fields: map[string]string{
			"prompt":  "hello",
			"workdir": ".",
		},
	})
	if err == nil {
		t.Fatalf("expected worker_type required error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "worker_type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskNewSubmit_CreatesTaskAndWritesAuditLog(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_new_submit",
		Fields: map[string]string{
			"worker_type": "exec",
			"prompt":      "echo hi",
			"workdir":     ".",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	var out struct {
		Task tasks.Task `json:"task"`
	}
	raw, err := json.Marshal(outAny)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v raw=%s", err, string(raw))
	}
	if strings.TrimSpace(out.Task.ID) == "" {
		t.Fatalf("expected created task id")
	}
	if out.Task.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q want %q", out.Task.Mode, tasks.ModeNew)
	}
	if strings.TrimSpace(string(out.Task.WorkerType)) != "exec" {
		t.Fatalf("worker_type=%q want %q", out.Task.WorkerType, "exec")
	}

	logs, err := deps.Tasks.ListLogs(ctx, out.Task.ID, 0, 200)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream == tasks.LogSystem && strings.Contains(l.Message, "task_new_submit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected task_new_submit action audit log")
	}
}

func TestTaskResumeSubmit_RejectsDeletedSession(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-secretary",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := deps.Tasks.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt,
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}
	if err := deps.Tasks.DeleteSession(ctx, tasks.SessionKeyForTask(task)); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	reg := NewRegistry(deps)
	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "task_resume_submit",
		Fields: map[string]string{
			"task_id": task.ID,
			"prompt":  "continue",
		},
	})
	if err == nil {
		t.Fatalf("expected deleted session error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "session is deleted") {
		t.Fatalf("unexpected error: %v", err)
	}

	list, err := deps.Tasks.ListTasksWithOptions(ctx, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("tasks=%d want 1", len(list))
	}
}

func TestTaskNewSubmit_DescriptionIncludesWorkerGuidance(t *testing.T) {
	desc := taskNewSubmitTool{}.DescriptionZH()
	wants := []string{
		"worker_type 仅允许 claude-code | codex | exec",
		"简单且追求速度 -> claude-code",
		"严肃/生产级迭代 -> codex",
		"不确定则先问",
		"自动推荐不使用 exec",
	}
	for _, want := range wants {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing guidance: %q", want)
		}
	}
}

func TestMissionContractUpsert_CreatesAndUpdatesByTaskID(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-contract-tool",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	createAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "mission_contract_upsert",
		Fields: map[string]string{
			"task_id":             task.ID,
			"goal":                "deliver autonomous loop",
			"constraints":         "run tests before commit,no destructive git",
			"acceptance_criteria": "go test ./... passes,pnpm test passes",
			"non_goals":           "rewrite all frontend",
		},
	})
	if err != nil {
		t.Fatalf("create contract via tool: %v", err)
	}
	var createOut struct {
		Contract tasks.MissionContract `json:"contract"`
	}
	rawCreate, _ := json.Marshal(createAny)
	if err := json.Unmarshal(rawCreate, &createOut); err != nil {
		t.Fatalf("decode create output: %v raw=%s", err, string(rawCreate))
	}
	if createOut.Contract.Revision != 1 {
		t.Fatalf("revision=%d, want 1", createOut.Contract.Revision)
	}
	if createOut.Contract.Key == "" {
		t.Fatalf("expected contract key")
	}
	if len(createOut.Contract.AcceptanceCriteria) != 2 {
		t.Fatalf("acceptance_criteria=%v", createOut.Contract.AcceptanceCriteria)
	}

	updateAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "mission_contract_upsert",
		Fields: map[string]string{
			"task_id":             task.ID,
			"goal":                "deliver autonomous loop safely",
			"acceptance_criteria": "go test ./... passes",
		},
	})
	if err != nil {
		t.Fatalf("update contract via tool: %v", err)
	}
	var updateOut struct {
		Contract tasks.MissionContract `json:"contract"`
	}
	rawUpdate, _ := json.Marshal(updateAny)
	if err := json.Unmarshal(rawUpdate, &updateOut); err != nil {
		t.Fatalf("decode update output: %v raw=%s", err, string(rawUpdate))
	}
	if updateOut.Contract.Revision != 2 {
		t.Fatalf("revision=%d, want 2", updateOut.Contract.Revision)
	}
	if updateOut.Contract.Goal != "deliver autonomous loop safely" {
		t.Fatalf("goal=%q", updateOut.Contract.Goal)
	}
}

func TestMissionContractUpsert_ConfirmOnlyByTaskID(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-contract-confirm",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "mission_contract_upsert",
		Fields: map[string]string{
			"task_id": task.ID,
			"goal":    "deliver autonomous loop",
		},
	}); err != nil {
		t.Fatalf("create contract via tool: %v", err)
	}

	confirmAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "mission_contract_upsert",
		Fields: map[string]string{
			"task_id": task.ID,
			"confirm": "true",
		},
	})
	if err != nil {
		t.Fatalf("confirm contract via tool: %v", err)
	}
	var confirmOut struct {
		Contract  tasks.MissionContract `json:"contract"`
		Confirmed bool                  `json:"confirmed"`
	}
	rawConfirm, _ := json.Marshal(confirmAny)
	if err := json.Unmarshal(rawConfirm, &confirmOut); err != nil {
		t.Fatalf("decode confirm output: %v raw=%s", err, string(rawConfirm))
	}
	if !confirmOut.Confirmed {
		t.Fatalf("confirmed=%v, want true", confirmOut.Confirmed)
	}

	key := tasks.SessionKeyForTask(task)
	confirmation, ok, err := deps.Tasks.GetMissionContractConfirmation(ctx, key)
	if err != nil {
		t.Fatalf("get mission contract confirmation: %v", err)
	}
	if !ok {
		t.Fatalf("expected mission contract confirmation for key=%s", key)
	}
	if confirmation.ConfirmedRevision != confirmOut.Contract.Revision {
		t.Fatalf("confirmed_revision=%d, want %d", confirmation.ConfirmedRevision, confirmOut.Contract.Revision)
	}
}

func TestMissionContractUpsert_RequiresGoal(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "mission_contract_upsert",
		Fields: map[string]string{
			"key": "c:conv-1",
		},
	})
	if err == nil {
		t.Fatalf("expected goal required error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "goal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectAutonomyPolicyUpsert_ByTaskID(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	workdir := filepath.Join(t.TempDir(), "proj-policy-tool")
	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    workdir,
		SessionID:  "sess-policy-tool",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "project_autonomy_policy_upsert",
		Fields: map[string]string{
			"task_id": task.ID,
			"mode":    "max",
		},
	})
	if err != nil {
		t.Fatalf("upsert project policy via tool: %v", err)
	}
	var out struct {
		Policy tasks.ProjectAutonomyPolicy `json:"policy"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if out.Policy.Mode != tasks.AutonomyModeMax {
		t.Fatalf("policy.mode=%q, want %q", out.Policy.Mode, tasks.AutonomyModeMax)
	}
	if out.Policy.ProjectKey != tasks.NormalizeProjectKey(workdir) {
		t.Fatalf("policy.project_key=%q, want %q", out.Policy.ProjectKey, tasks.NormalizeProjectKey(workdir))
	}
}

func TestExecutionPlanLoopSubmit_RunsAndPersistsProgress(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-loop-tool",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-loop-tool",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := deps.Tasks.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt,
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	contractKey := tasks.ConversationKey(task.ConversationID)
	if _, err := deps.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  contractKey,
		Goal: "deliver autonomous loop",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := deps.Tasks.ConfirmMissionContract(ctx, contractKey); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "execution_plan_loop_submit",
		Fields: map[string]string{
			"task_id":          task.ID,
			"max_iterations":   "1",
			"iteration_prompt": "continue",
		},
	})
	if err != nil {
		t.Fatalf("run execution plan loop: %v", err)
	}
	var out struct {
		IterationsExecuted int                      `json:"iterations_executed"`
		State              tasks.ExecutionPlanState `json:"state"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if out.IterationsExecuted != 1 {
		t.Fatalf("iterations_executed=%d, want 1", out.IterationsExecuted)
	}
	if out.State.Iteration != 1 {
		t.Fatalf("state.iteration=%d, want 1", out.State.Iteration)
	}
	if strings.TrimSpace(out.State.PlanJSON) == "" {
		t.Fatalf("expected non-empty plan_json")
	}
}

func TestExecutionPlanLoopSubmit_EmitsHumanHandoffAtLimit(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-loop-limit-tool",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-loop-limit-tool",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := deps.Tasks.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt,
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	contractKey := tasks.ConversationKey(task.ConversationID)
	if _, err := deps.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  contractKey,
		Goal: "deliver autonomous loop",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := deps.Tasks.ConfirmMissionContract(ctx, contractKey); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	firstAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "execution_plan_loop_submit",
		Fields: map[string]string{
			"task_id":              task.ID,
			"max_iterations":       "1",
			"max_total_iterations": "1",
		},
	})
	if err != nil {
		t.Fatalf("run first loop: %v", err)
	}
	var first struct {
		LastTaskID string `json:"last_task_id"`
	}
	rawFirst, _ := json.Marshal(firstAny)
	if err := json.Unmarshal(rawFirst, &first); err != nil {
		t.Fatalf("decode first output: %v raw=%s", err, string(rawFirst))
	}
	if strings.TrimSpace(first.LastTaskID) == "" {
		t.Fatalf("expected last_task_id")
	}
	if err := deps.Tasks.FinishTask(ctx, first.LastTaskID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "step-1 done",
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt,
	}); err != nil {
		t.Fatalf("finish loop task: %v", err)
	}

	secondAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "execution_plan_loop_submit",
		Fields: map[string]string{
			"task_id":              task.ID,
			"max_iterations":       "1",
			"max_total_iterations": "1",
		},
	})
	if err != nil {
		t.Fatalf("run second loop: %v", err)
	}
	var second struct {
		LimitReached bool `json:"limit_reached"`
		Handoff      struct {
			Action           string   `json:"action"`
			Summary          string   `json:"summary"`
			Blockers         []string `json:"blockers"`
			SuggestedActions []string `json:"suggested_actions"`
		} `json:"handoff"`
	}
	rawSecond, _ := json.Marshal(secondAny)
	if err := json.Unmarshal(rawSecond, &second); err != nil {
		t.Fatalf("decode second output: %v raw=%s", err, string(rawSecond))
	}
	if !second.LimitReached {
		t.Fatalf("expected limit_reached=true")
	}
	if second.Handoff.Action != "human_handoff" {
		t.Fatalf("handoff.action=%q, want human_handoff", second.Handoff.Action)
	}
	if strings.TrimSpace(second.Handoff.Summary) == "" {
		t.Fatalf("expected handoff summary")
	}
}

func TestRollbackPlaybookGenerate_UsesProofReferences(t *testing.T) {
	ctx, deps := newDepsForToolsTest(t)
	reg := NewRegistry(deps)

	task, err := deps.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-rollback-playbook",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := deps.Tasks.CreateRollbackProof(ctx, tasks.CreateRollbackProofInput{
		TaskID:     task.ID,
		ActionType: "git.remote",
		ActionRef:  "push-main",
		ProofType:  "git_tag",
		ProofRef:   "refs/tags/snapshot-20260212",
		Detail:     json.RawMessage(`{"tag":"snapshot-20260212"}`),
	}); err != nil {
		t.Fatalf("create rollback proof #1: %v", err)
	}
	if _, err := deps.Tasks.CreateRollbackProof(ctx, tasks.CreateRollbackProofInput{
		TaskID:     task.ID,
		ActionType: "git.remote",
		ActionRef:  "push-main",
		ProofType:  "snapshot",
		ProofRef:   "snapshot://rev-1",
		Detail:     json.RawMessage(`{"workspace":"rev-1"}`),
	}); err != nil {
		t.Fatalf("create rollback proof #2: %v", err)
	}

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "rollback_playbook_generate",
		Fields: map[string]string{
			"task_id": task.ID,
		},
	})
	if err != nil {
		t.Fatalf("generate rollback playbook: %v", err)
	}
	var out struct {
		TaskID   string `json:"task_id"`
		Playbook string `json:"playbook"`
		Steps    []struct {
			ProofRef  string `json:"proof_ref"`
			ProofType string `json:"proof_type"`
		} `json:"steps"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if strings.TrimSpace(out.TaskID) != task.ID {
		t.Fatalf("task_id=%q, want %q", out.TaskID, task.ID)
	}
	if len(out.Steps) < 2 {
		t.Fatalf("steps=%v, want at least 2", out.Steps)
	}
	if !strings.Contains(out.Playbook, "snapshot://rev-1") {
		t.Fatalf("playbook missing proof ref: %q", out.Playbook)
	}
	if !strings.Contains(out.Playbook, "refs/tags/snapshot-20260212") {
		t.Fatalf("playbook missing proof ref: %q", out.Playbook)
	}
}

func strconvFormatInt(v int64) string {
	return fmt.Sprintf("%d", v)
}

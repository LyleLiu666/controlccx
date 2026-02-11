package taskops

import (
	"errors"
	"strings"

	"controlccx/internal/tasks"
	"controlccx/internal/worktree"
)

type MutationAction string

const (
	ActionTaskCreate             MutationAction = "task.create"
	ActionTaskResume             MutationAction = "task.resume"
	ActionTaskRehydrate          MutationAction = "task.rehydrate"
	ActionTaskEnterUnsafe        MutationAction = "task.enter_unsafe"
	ActionSessionContinue        MutationAction = "session.continue"
	ActionSessionPreemptContinue MutationAction = "session.preempt_continue"
)

type MutationResult struct {
	OK     bool           `json:"ok"`
	Action MutationAction `json:"action"`
	Task   *tasks.Task    `json:"task,omitempty"`
	Queue  *QueueAck      `json:"queue,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

func NewTaskMutationResult(action MutationAction, task tasks.Task) MutationResult {
	copy := task
	return MutationResult{
		OK:     true,
		Action: action,
		Task:   &copy,
	}
}

func NewQueueMutationResult(action MutationAction, queue QueueAck) MutationResult {
	copy := queue
	return MutationResult{
		OK:     true,
		Action: action,
		Queue:  &copy,
	}
}

type MutationErrorCode string

const (
	MutationErrorInvalidArgument     MutationErrorCode = "invalid_argument"
	MutationErrorNotFound            MutationErrorCode = "not_found"
	MutationErrorWorkdirBusy         MutationErrorCode = "workdir_busy"
	MutationErrorSessionTaskInFlight MutationErrorCode = "session_task_in_flight"
	MutationErrorRunnerUnavailable   MutationErrorCode = "runner_unavailable"
	MutationErrorUnsupported         MutationErrorCode = "unsupported"
	MutationErrorInternal            MutationErrorCode = "internal"
)

type MutationProblem struct {
	OK      bool              `json:"ok"`
	Error   MutationErrorCode `json:"error"`
	Message string            `json:"message"`
	Hint    string            `json:"hint,omitempty"`
	Details map[string]any    `json:"details,omitempty"`
	Status  int               `json:"-"`
}

type MutationError struct {
	Problem MutationProblem
	Err     error
}

func (e *MutationError) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.Problem.Message)
	if msg != "" {
		return msg
	}
	if e.Err != nil {
		return strings.TrimSpace(e.Err.Error())
	}
	return "mutation failed"
}

func (e *MutationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newMutationError(status int, code MutationErrorCode, message string, hint string, details map[string]any, err error) *MutationError {
	return &MutationError{
		Problem: MutationProblem{
			OK:      false,
			Error:   code,
			Message: strings.TrimSpace(message),
			Hint:    strings.TrimSpace(hint),
			Details: details,
			Status:  status,
		},
		Err: err,
	}
}

func ParseMutationError(err error) MutationProblem {
	const runnerHint = "restart the runner daemon (controlccx-runnerd)"

	if err == nil {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorInternal,
			Message: "unknown error",
			Status:  500,
		}
	}

	var mutationErr *MutationError
	if errors.As(err, &mutationErr) {
		out := mutationErr.Problem
		if out.Status == 0 {
			out.Status = 500
		}
		out.OK = false
		if strings.TrimSpace(out.Message) == "" {
			out.Message = strings.TrimSpace(err.Error())
		}
		return out
	}

	var busy *tasks.WorkDirBusyError
	if errors.As(err, &busy) {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorWorkdirBusy,
			Message: strings.TrimSpace(err.Error()),
			Status:  409,
			Details: map[string]any{
				"workdir":          strings.TrimSpace(busy.WorkDir),
				"existing_task_id": strings.TrimSpace(busy.ExistingTaskID),
				"existing_status":  strings.TrimSpace(string(busy.ExistingStatus)),
			},
		}
	}

	var runnerErr *RunnerUnavailableError
	if errors.As(err, &runnerErr) {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorRunnerUnavailable,
			Message: strings.TrimSpace(err.Error()),
			Hint:    runnerHint,
			Status:  503,
			Details: map[string]any{
				"task_id": strings.TrimSpace(runnerErr.TaskID),
			},
		}
	}

	var tooLarge *worktree.UntrackedTooLargeError
	if errors.As(err, &tooLarge) {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorInvalidArgument,
			Message: strings.TrimSpace(err.Error()),
			Status:  422,
			Details: map[string]any{
				"reason":    "worktree_untracked_too_large",
				"files":     tooLarge.Files,
				"bytes":     tooLarge.Bytes,
				"max_files": tooLarge.MaxFiles,
				"max_bytes": tooLarge.MaxBytes,
				"largest":   tooLarge.Largest,
			},
		}
	}

	msg := strings.TrimSpace(err.Error())
	if strings.HasPrefix(msg, "session_task_in_flight:") {
		existingID := ""
		existingStatus := ""
		for _, part := range strings.Fields(msg) {
			switch {
			case strings.HasPrefix(part, "existing_task_id="):
				existingID = strings.TrimPrefix(part, "existing_task_id=")
			case strings.HasPrefix(part, "existing_status="):
				existingStatus = strings.TrimPrefix(part, "existing_status=")
			}
		}
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorSessionTaskInFlight,
			Message: "session already has an in-flight task",
			Status:  409,
			Details: map[string]any{
				"existing_task_id": strings.TrimSpace(existingID),
				"existing_status":  strings.TrimSpace(existingStatus),
			},
		}
	}

	if isMutationNotFoundMessage(msg) {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorNotFound,
			Message: msg,
			Status:  404,
		}
	}

	if strings.Contains(msg, "only supported") ||
		strings.Contains(msg, "does not support") ||
		strings.Contains(msg, "cannot continue") {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorUnsupported,
			Message: msg,
			Status:  400,
		}
	}

	if isMutationInvalidArgumentMessage(msg) {
		return MutationProblem{
			OK:      false,
			Error:   MutationErrorInvalidArgument,
			Message: msg,
			Status:  400,
		}
	}

	return MutationProblem{
		OK:      false,
		Error:   MutationErrorInternal,
		Message: msg,
		Status:  500,
	}
}

func isMutationNotFoundMessage(msg string) bool {
	if msg == "" {
		return false
	}
	if msg == "session not found" || msg == "tasks: not found" {
		return true
	}
	if strings.HasPrefix(msg, "task not found:") {
		return true
	}
	if strings.HasPrefix(msg, "tasks: ") && strings.HasSuffix(msg, " not found") {
		return true
	}
	return false
}

func isMutationInvalidArgumentMessage(msg string) bool {
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "required") {
		return true
	}
	if strings.Contains(msg, "unknown tool id") {
		return true
	}
	if strings.Contains(msg, "invalid session key") {
		return true
	}
	if strings.HasPrefix(msg, "invalid ") {
		return true
	}
	if strings.HasPrefix(msg, "tasks: invalid ") || strings.HasPrefix(msg, "taskops: invalid ") {
		return true
	}
	if strings.Contains(msg, "workdir_strategy=worktree") ||
		strings.Contains(msg, "worktree_untracked must be one of") ||
		strings.Contains(msg, "conversation_id must be a UUID") ||
		strings.Contains(msg, "task has no session_id") ||
		strings.Contains(msg, "task has no conversation_id") {
		return true
	}
	return false
}

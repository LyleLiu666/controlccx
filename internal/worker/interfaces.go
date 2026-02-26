package worker

import (
	"context"
	"io"

	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

// TaskStore defines the subset of tasks.Store that Manager depends on.
// Extracting this interface allows mock-based unit testing of Manager.run()
// without requiring a real SQLite database.
type TaskStore interface {
	// Task lifecycle
	CreateTask(ctx context.Context, in tasks.CreateTaskInput) (tasks.Task, error)
	GetTask(ctx context.Context, id string) (tasks.Task, error)
	SetRunning(ctx context.Context, id string) error
	SetBlocked(ctx context.Context, id string) error
	SetAwaitingApproval(ctx context.Context, id string) error
	SetRunningStatus(ctx context.Context, id string) error
	FinishTask(ctx context.Context, id string, in tasks.FinishTaskInput) error
	TouchTask(ctx context.Context, id string) error

	// Task metadata
	SetWarning(ctx context.Context, id string, warning string) error
	SetSessionID(ctx context.Context, id, sessionID string) error
	SetInvocation(ctx context.Context, taskID string, cmd string, args []string, dir string, envKeys []string) error

	// Logs
	AppendLog(ctx context.Context, taskID string, stream tasks.LogStream, message string) (tasks.LogEntry, error)
	ListLogs(ctx context.Context, taskID string, afterID int64, limit int) ([]tasks.LogEntry, error)

	// Approval flow
	CreateApprovalRequest(ctx context.Context, in tasks.CreateApprovalRequestInput) (tasks.ApprovalRequest, error)
	UpdateApprovalRequestDecision(ctx context.Context, approvalID string, in tasks.UpdateApprovalRequestDecisionInput) error

	// Queue management
	DequeueNextWaitingForWorkdir(ctx context.Context, workdir string) (tasks.Task, bool, error)

	// Project context
	GetProjectContext(ctx context.Context) (tasks.ProjectContext, bool, error)
}

// EventPublisher defines the subset of events.Hub that Manager depends on.
type EventPublisher interface {
	Publish(evt events.Event)
}

// Compile-time checks: ensure the concrete types satisfy our interfaces.
// SpawnOpts defines the environment and parameters for spawning a child process.
type SpawnOpts struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
}

// ManagedRun represents a running, managed child process.
type ManagedRun interface {
	PID() int
	Stdout() io.Reader
	Stderr() io.Reader
	Stdin() io.WriteCloser
	Wait() (exitCode int, err error)
	Cancel() error
}

// ProcessRunner is responsible for spawning child processes.
type ProcessRunner interface {
	Spawn(ctx context.Context, opts SpawnOpts) (ManagedRun, error)
}

// ProtocolContext provides callbacks to the Manager for protocol handlers.
type ProtocolContext interface {
	Task() tasks.Task
	Context() context.Context

	AppendLog(stream tasks.LogStream, message string)
	SetSessionID(sid string)
	PublishTaskUpdated()

	SetResumeFailure(msg string)
	SetBlocked(reason string)
	MarkToolError()

	// Protocol-specific callbacks (to be abstracted further in the future if needed)
	OnClaudeCanUseTool(toolName string, input map[string]any, suggestions []byte, toolUseID string) (any, error)
	OnCodexServerRequest(id []byte, method string, params []byte)
}

// ProtocolHandler is responsible for parsing standard streams from an agent process.
type ProtocolHandler interface {
	ConsumeStdout(pctx ProtocolContext, r io.Reader) error
	ConsumeStderr(pctx ProtocolContext, r io.Reader) error
	CloseStdin() error
}

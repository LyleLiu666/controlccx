package tasks

import "time"

type Status string

const (
	StatusQueued           Status = "queued"
	StatusWaiting          Status = "waiting"
	StatusRunning          Status = "running"
	StatusAwaitingApproval Status = "awaiting_approval"
	StatusSucceeded        Status = "succeeded"
	StatusFailed           Status = "failed"
	StatusCanceled         Status = "canceled"
	StatusInterrupted      Status = "interrupted"
	StatusBlocked          Status = "blocked"
)

type WorkerType string

const (
	WorkerClaudeCode WorkerType = "claude-code"
	WorkerCodex      WorkerType = "codex"
	WorkerExec       WorkerType = "exec"
)

type Mode string

const (
	ModeNew    Mode = "new"
	ModeResume Mode = "resume"
)

type Task struct {
	ID                    string     `json:"id"`
	ConversationID        string     `json:"conversation_id"`
	WorkerType            WorkerType `json:"worker_type"`
	Mode                  Mode       `json:"mode"`
	Status                Status     `json:"status"`
	UnsafeAutomation      bool       `json:"unsafe_automation,omitempty"`
	WorkDirStrategy       string     `json:"workdir_strategy,omitempty"`
	SafetyPreset          string     `json:"safety_preset,omitempty"`
	TaskIntent            string     `json:"task_intent,omitempty"`
	CodexSandbox          string     `json:"codex_sandbox,omitempty"`
	CodexApprovalPolicy   string     `json:"codex_approval_policy,omitempty"`
	CodexSearch           bool       `json:"codex_search,omitempty"`
	ClaudePermissionMode  string     `json:"claude_permission_mode,omitempty"`
	ClaudeSandbox         bool       `json:"claude_sandbox,omitempty"`
	ClaudeWebFetchDomains []string   `json:"claude_webfetch_domains,omitempty"`
	Prompt                string     `json:"prompt"`
	WorkDir               string     `json:"workdir"`
	BaseWorkDir           string     `json:"base_workdir,omitempty"`
	WorktreeDir           string     `json:"worktree_dir,omitempty"`
	WorktreeBranch        string     `json:"worktree_branch,omitempty"`
	SessionID             string     `json:"session_id"`
	SessionTitle          string     `json:"session_title,omitempty"`
	SessionDeletedAt      *time.Time `json:"session_deleted_at,omitempty"`
	Warning               string     `json:"warning"`
	Error                 string     `json:"error"`
	ExitCode              *int       `json:"exit_code,omitempty"`
	FinishReason          string     `json:"finish_reason,omitempty"`
	SuggestedTests        []string   `json:"suggested_tests,omitempty"`
	StderrCount           int        `json:"stderr_count"`
	KeywordCount          int        `json:"keyword_count"`
	Score                 int        `json:"score"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	FinishedAt            *time.Time `json:"finished_at,omitempty"`
}

type LogStream string

const (
	LogStdout    LogStream = "stdout"
	LogStderr    LogStream = "stderr"
	LogSystem    LogStream = "system"
	LogAssistant LogStream = "assistant"
)

type LogEntry struct {
	ID      int64     `json:"id"`
	TaskID  string    `json:"task_id"`
	Time    time.Time `json:"time"`
	Stream  LogStream `json:"stream"`
	Message string    `json:"message"`
}

type CreateTaskInput struct {
	WorkerType        WorkerType `json:"worker_type"`
	Mode              Mode       `json:"mode"`
	ConversationID    string     `json:"conversation_id,omitempty"`
	IdempotencyKey    string     `json:"idempotency_key,omitempty"`
	WorkDirStrategy   string     `json:"workdir_strategy,omitempty"`
	BaseWorkDir       string     `json:"base_workdir,omitempty"`
	WorktreeDir       string     `json:"worktree_dir,omitempty"`
	WorktreeBranch    string     `json:"worktree_branch,omitempty"`
	WorktreeUntracked string     `json:"worktree_untracked,omitempty"`
	UnsafeAutomation  bool       `json:"unsafe_automation,omitempty"`
	// SafetyEnvelope is an optional autopilot hint (UI-level “one-time unlock”).
	// It is not persisted; it only affects server-side defaults when run safety options are omitted.
	SafetyEnvelope        string   `json:"safety_envelope,omitempty"`
	SafetyPreset          string   `json:"safety_preset,omitempty"`
	TaskIntent            string   `json:"task_intent,omitempty"`
	CodexSandbox          string   `json:"codex_sandbox,omitempty"`
	CodexApprovalPolicy   string   `json:"codex_approval_policy,omitempty"`
	CodexSearch           bool     `json:"codex_search,omitempty"`
	ClaudePermissionMode  string   `json:"claude_permission_mode,omitempty"`
	ClaudeSandbox         bool     `json:"claude_sandbox,omitempty"`
	ClaudeWebFetchDomains []string `json:"claude_webfetch_domains,omitempty"`
	Prompt                string   `json:"prompt"`
	WorkDir               string   `json:"workdir"`
	SessionID             string   `json:"session_id,omitempty"`
	Warning               string   `json:"warning,omitempty"`
}

type FinishTaskInput struct {
	Status     Status
	ExitCode   *int
	Error      string
	SessionID  string
	FinishedAt time.Time
}

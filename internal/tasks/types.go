package tasks

import "time"

type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusCanceled    Status = "canceled"
	StatusInterrupted Status = "interrupted"
	StatusBlocked     Status = "blocked"
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
	ID           string     `json:"id"`
	WorkerType   WorkerType `json:"worker_type"`
	Mode         Mode       `json:"mode"`
	Status       Status     `json:"status"`
	UnsafeAutomation bool   `json:"unsafe_automation,omitempty"`
	Prompt       string     `json:"prompt"`
	WorkDir      string     `json:"workdir"`
	SessionID    string     `json:"session_id"`
	Warning      string     `json:"warning"`
	Error        string     `json:"error"`
	ExitCode     *int       `json:"exit_code,omitempty"`
	StderrCount  int        `json:"stderr_count"`
	KeywordCount int        `json:"keyword_count"`
	Score        int        `json:"score"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
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
	WorkerType WorkerType `json:"worker_type"`
	Mode       Mode       `json:"mode"`
	UnsafeAutomation bool `json:"unsafe_automation,omitempty"`
	Prompt     string     `json:"prompt"`
	WorkDir    string     `json:"workdir"`
	SessionID  string     `json:"session_id,omitempty"`
	Warning    string     `json:"warning,omitempty"`
}

type FinishTaskInput struct {
	Status     Status
	ExitCode   *int
	Error      string
	SessionID  string
	FinishedAt time.Time
}

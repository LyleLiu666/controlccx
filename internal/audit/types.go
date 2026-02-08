package audit

import (
	"errors"
	"time"

	"controlccx/internal/tasks"
)

type Source string

const (
	SourceTaskLog              Source = "task_log"
	SourceTaskTrace            Source = "task_trace"
	SourceSecretaryEvent       Source = "secretary_event"
	SourceSecretaryCompression Source = "secretary_compression"
	SourceSecretaryChat        Source = "secretary_chat"
)

var (
	ErrInvalidSource = errors.New("audit: invalid source")
	ErrInvalidCursor = errors.New("audit: invalid cursor")
	ErrEntryNotFound = errors.New("audit: entry not found")
)

var allSources = []Source{
	SourceTaskLog,
	SourceTaskTrace,
	SourceSecretaryEvent,
	SourceSecretaryCompression,
	SourceSecretaryChat,
}

func isKnownSource(s Source) bool {
	for _, item := range allSources {
		if item == s {
			return true
		}
	}
	return false
}

type Query struct {
	Sources []Source
	Q       string
	From    time.Time
	To      time.Time
	TaskID  string
	RunID   string
	Streams []tasks.LogStream
	Limit   int
	Cursor  string
}

type QueryResult struct {
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type Entry struct {
	ID         string    `json:"id"`
	Source     Source    `json:"source"`
	Time       time.Time `json:"time"`
	TaskID     string    `json:"task_id,omitempty"`
	RunID      string    `json:"run_id,omitempty"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	RawPreview string    `json:"raw_preview"`
}

type EntryDetail struct {
	Entry
	Raw  string         `json:"raw"`
	Meta map[string]any `json:"meta,omitempty"`
}

type SourceInfo struct {
	Source          Source            `json:"source"`
	Label           string            `json:"label"`
	DefaultEnabled  bool              `json:"default_enabled"`
	SupportsTaskID  bool              `json:"supports_task_id"`
	SupportsRunID   bool              `json:"supports_run_id"`
	SupportsStreams bool              `json:"supports_streams"`
	DefaultStreams  []tasks.LogStream `json:"default_streams,omitempty"`
}

type Options struct {
	Now       func() time.Time
	Retention RetentionOptions
}

type RetentionOptions struct {
	Days          int
	MaxRows       int
	GCInterval    time.Duration
	StartupRunGC  bool
	PreviewRunCap int
}

type RetentionStatus struct {
	Days              int       `json:"days"`
	MaxRowsPerSource  int       `json:"max_rows_per_source"`
	GCIntervalSeconds int64     `json:"gc_interval_seconds"`
	LastRun           *GCStatus `json:"last_run,omitempty"`
}

type GCStatus struct {
	RunAt      time.Time        `json:"run_at"`
	DurationMs int64            `json:"duration_ms"`
	Results    []GCSourceResult `json:"results"`
}

type GCSourceResult struct {
	Source         Source `json:"source"`
	Table          string `json:"table"`
	DeletedByAge   int64  `json:"deleted_by_age"`
	DeletedByCount int64  `json:"deleted_by_count"`
	Error          string `json:"error,omitempty"`
}

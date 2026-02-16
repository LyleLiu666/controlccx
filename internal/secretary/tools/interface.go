package tools

import (
	"context"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/skills"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

type Tool interface {
	Name() string
	DescriptionZH() string
	Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error)
}

type ParamDescriber interface {
	Params() []string
	Required() []string
	AnyOfRequired() [][]string
}

type Deps struct {
	Tasks     *tasks.Store
	Skills    *skills.Service
	Ops       *taskops.Service
	Scheduler Scheduler
	FSRoots   []string
}

type Descriptor struct {
	Name          string
	DescriptionZH string
	Params        []string
	Required      []string
	AnyOfRequired [][]string
}

type ScheduleState string

const (
	ScheduleStateActive   ScheduleState = "active"
	ScheduleStateCanceled ScheduleState = "canceled"
	ScheduleStateExpired  ScheduleState = "expired"
	ScheduleStateDone     ScheduleState = "done"
)

type SchedulerCreateRequest struct {
	ToolName       string
	ToolFields     map[string]string
	ToolFieldsJSON string
	IntervalSec    int
	TTLSec         int
	AllowWrite     bool
}

type ScheduleInfo struct {
	ID               string        `json:"id"`
	TargetToolName   string        `json:"target_tool_name"`
	TargetFieldsJSON string        `json:"target_fields_json"`
	IntervalSec      int           `json:"interval_sec"`
	TTLSec           int           `json:"ttl_sec"`
	AllowWrite       bool          `json:"allow_write"`
	State            ScheduleState `json:"state"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	NextTickAt       time.Time     `json:"next_tick_at,omitempty"`
	TickNo           int           `json:"tick_no"`
	Running          bool          `json:"running"`
	Pending          bool          `json:"pending"`
}

type Scheduler interface {
	CreateSchedule(ctx context.Context, req SchedulerCreateRequest) (ScheduleInfo, error)
	ListSchedules(ctx context.Context) ([]ScheduleInfo, error)
	CancelSchedule(ctx context.Context, scheduleID string) (ScheduleInfo, error)
}

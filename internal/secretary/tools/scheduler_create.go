package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/agentsdk"
)

type schedulerCreateTool struct{}

func (schedulerCreateTool) Name() string { return "scheduler_create" }

func (schedulerCreateTool) DescriptionZH() string {
	return "创建定时调度。参数：tool_name（或 target_tool_name，必填，目标工具名）、tool_fields_json（必填，JSON object string）、interval_sec（可选，默认10，最大60）、ttl_sec（可选，默认300）、allow_write（可选，默认false）。"
}

func (schedulerCreateTool) Params() []string {
	return []string{
		"tool_name",
		"target_tool_name",
		"name",
		"tool_fields_json",
		"interval_sec",
		"ttl_sec",
		"allow_write",
	}
}

func (schedulerCreateTool) Required() []string { return []string{"tool_fields_json"} }

func (schedulerCreateTool) AnyOfRequired() [][]string {
	return [][]string{{"tool_name", "target_tool_name", "name"}}
}

func (schedulerCreateTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	scheduler, err := requireScheduler(deps)
	if err != nil {
		return nil, err
	}
	targetToolName := parseSchedulerTargetToolName(call.Fields)
	if targetToolName == "" {
		return nil, errors.New("tool_name is required")
	}
	targetFields, targetFieldsJSON, err := parseSchedulerToolFieldsJSON(call.Fields["tool_fields_json"])
	if err != nil {
		return nil, err
	}

	intervalSec := parseInt(call.Fields["interval_sec"], 10)
	if intervalSec <= 0 {
		intervalSec = 10
	}
	if intervalSec > 60 {
		return nil, fmt.Errorf("interval_sec must be <= 60")
	}

	ttlSec := parseInt(call.Fields["ttl_sec"], 300)
	if ttlSec <= 0 {
		ttlSec = 300
	}

	allowWrite := parseBool(call.Fields["allow_write"])
	info, err := scheduler.CreateSchedule(withBackgroundContext(ctx), SchedulerCreateRequest{
		ToolName:       strings.TrimSpace(targetToolName),
		ToolFields:     copyStringMap(targetFields),
		ToolFieldsJSON: strings.TrimSpace(targetFieldsJSON),
		IntervalSec:    intervalSec,
		TTLSec:         ttlSec,
		AllowWrite:     allowWrite,
	})
	if err != nil {
		return nil, err
	}
	return scheduleInfoToResult(info), nil
}

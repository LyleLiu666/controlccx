package tools

import (
	"context"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
)

type schedulerStub struct {
	createReqs []SchedulerCreateRequest
	createOut  ScheduleInfo
	createErr  error

	listOut []ScheduleInfo
	listErr error

	cancelReqs []string
	cancelOut  ScheduleInfo
	cancelErr  error
}

func (s *schedulerStub) CreateSchedule(ctx context.Context, req SchedulerCreateRequest) (ScheduleInfo, error) {
	_ = ctx
	s.createReqs = append(s.createReqs, req)
	if s.createErr != nil {
		return ScheduleInfo{}, s.createErr
	}
	return s.createOut, nil
}

func (s *schedulerStub) ListSchedules(ctx context.Context) ([]ScheduleInfo, error) {
	_ = ctx
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]ScheduleInfo(nil), s.listOut...), nil
}

func (s *schedulerStub) CancelSchedule(ctx context.Context, scheduleID string) (ScheduleInfo, error) {
	_ = ctx
	s.cancelReqs = append(s.cancelReqs, scheduleID)
	if s.cancelErr != nil {
		return ScheduleInfo{}, s.cancelErr
	}
	return s.cancelOut, nil
}

func TestSchedulerCreateTool_ParsesDefaultsAndBounds(t *testing.T) {
	ctx := context.Background()
	stub := &schedulerStub{createOut: ScheduleInfo{ID: "s-1", State: ScheduleStateActive}}
	reg := NewRegistry(Deps{Scheduler: stub})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "scheduler_create",
		Fields: map[string]string{
			"target_tool_name": "tasks_list",
			"tool_fields_json": `{"limit":5,"include_deleted":true}`,
			"conversation_id": "conv-a",
		},
	})
	if err != nil {
		t.Fatalf("scheduler_create default parse: %v", err)
	}
	if len(stub.createReqs) != 1 {
		t.Fatalf("create req count=%d want 1", len(stub.createReqs))
	}
	req := stub.createReqs[0]
	if req.ToolName != "tasks_list" {
		t.Fatalf("tool_name=%q want tasks_list", req.ToolName)
	}
	if req.ToolFields["limit"] != "5" {
		t.Fatalf("limit=%q want 5", req.ToolFields["limit"])
	}
	if req.ToolFields["include_deleted"] != "true" {
		t.Fatalf("include_deleted=%q want true", req.ToolFields["include_deleted"])
	}
	if req.ConversationID != "conv-a" {
		t.Fatalf("conversation_id=%q want conv-a", req.ConversationID)
	}
	if req.IntervalSec != 10 {
		t.Fatalf("interval_sec=%d want 10", req.IntervalSec)
	}
	if req.TTLSec != 300 {
		t.Fatalf("ttl_sec=%d want 300", req.TTLSec)
	}
	if req.AllowWrite {
		t.Fatalf("allow_write=%v want false", req.AllowWrite)
	}

	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("output type=%T want map", outAny)
	}
	if out["schedule_id"] != "s-1" {
		t.Fatalf("schedule_id=%v want s-1", out["schedule_id"])
	}

	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "scheduler_create",
		Fields: map[string]string{
			"target_tool_name": "tasks_list",
			"tool_fields_json": `{"limit":1}`,
			"interval_sec":     "61",
		},
	})
	if err == nil {
		t.Fatalf("expected interval bound error")
	}

	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "scheduler_create",
		Fields: map[string]string{
			"target_tool_name": "tasks_list",
			"tool_fields_json": `oops`,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestSchedulerListAndCancelTools(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	stub := &schedulerStub{
		listOut: []ScheduleInfo{{
			ID:             "s-1",
			TargetToolName: "tasks_count",
			IntervalSec:    10,
			TTLSec:         300,
			AllowWrite:     false,
			State:          ScheduleStateActive,
			CreatedAt:      now,
			ExpiresAt:      now.Add(5 * time.Minute),
			NextTickAt:     now.Add(10 * time.Second),
		}},
		cancelOut: ScheduleInfo{ID: "s-1", State: ScheduleStateCanceled},
	}
	reg := NewRegistry(Deps{Scheduler: stub})

	listAny, err := reg.Execute(ctx, agentsdk.ToolCall{Name: "scheduler_list"})
	if err != nil {
		t.Fatalf("scheduler_list: %v", err)
	}
	list := listAny.(map[string]any)
	schedules, ok := list["schedules"].([]ScheduleInfo)
	if !ok {
		t.Fatalf("schedules type=%T want []ScheduleInfo", list["schedules"])
	}
	if len(schedules) != 1 || schedules[0].ID != "s-1" {
		t.Fatalf("unexpected schedules=%+v", schedules)
	}

	cancelAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "scheduler_cancel",
		Fields: map[string]string{
			"schedule_id": "s-1",
		},
	})
	if err != nil {
		t.Fatalf("scheduler_cancel: %v", err)
	}
	cancel := cancelAny.(map[string]any)
	if cancel["schedule_id"] != "s-1" {
		t.Fatalf("schedule_id=%v want s-1", cancel["schedule_id"])
	}
	if cancel["state"] != string(ScheduleStateCanceled) {
		t.Fatalf("state=%v want canceled", cancel["state"])
	}
}

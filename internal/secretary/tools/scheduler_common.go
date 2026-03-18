package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func requireScheduler(deps Deps) (Scheduler, error) {
	if deps.Scheduler == nil {
		return nil, errors.New("scheduler not configured")
	}
	return deps.Scheduler, nil
}

func parseSchedulerTargetToolName(fields map[string]string) string {
	if v := strings.TrimSpace(fields["tool_name"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(fields["target_tool_name"]); v != "" {
		return v
	}
	return strings.TrimSpace(fields["name"])
}

func parseSchedulerToolFieldsJSON(raw string) (map[string]string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", errors.New("tool_fields_json is required")
	}
	var in map[string]any
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, "", fmt.Errorf("tool_fields_json must be a valid JSON object string: %w", err)
	}
	if in == nil {
		in = map[string]any{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = stringifySchedulerField(v)
	}
	canonicalBytes, err := json.Marshal(in)
	if err != nil {
		return nil, "", fmt.Errorf("tool_fields_json marshal: %w", err)
	}
	return out, string(canonicalBytes), nil
}

func stringifySchedulerField(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func scheduleInfoToResult(info ScheduleInfo) map[string]any {
	out := map[string]any{
		"schedule_id":        strings.TrimSpace(info.ID),
		"state":              string(info.State),
		"target_tool_name":   strings.TrimSpace(info.TargetToolName),
		"target_fields_json": strings.TrimSpace(info.TargetFieldsJSON),
		"conversation_id":    strings.TrimSpace(info.ConversationID),
		"interval_sec":       info.IntervalSec,
		"ttl_sec":            info.TTLSec,
		"allow_write":        info.AllowWrite,
		"created_at":         info.CreatedAt,
		"expires_at":         info.ExpiresAt,
		"tick_no":            info.TickNo,
		"running":            info.Running,
		"pending":            info.Pending,
	}
	if !info.NextTickAt.IsZero() {
		out["next_tick_at"] = info.NextTickAt
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}

func withBackgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

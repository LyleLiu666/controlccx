package secretary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
)

type EventStore struct {
	db  *sql.DB
	now func() time.Time
}

type StoredEvent struct {
	ID        int64
	Time      time.Time
	RunID     string
	Kind      agentsdk.EventKind
	Protocol  string
	Step      int
	EventJSON string
}

func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db, now: time.Now}
}

func (s *EventStore) Append(ctx context.Context, runID string, ev agentsdk.Event) error {
	if s == nil || s.db == nil {
		return errors.New("secretary: event store not initialized")
	}
	rid := strings.TrimSpace(runID)
	if rid == "" {
		return errors.New("secretary: event store: run_id is required")
	}

	ts := ev.Time
	if ts.IsZero() {
		ts = s.now()
	}
	ts = ts.UTC()

	sanitized := sanitizeEventForStorage(ev)
	eventJSON, err := json.Marshal(sanitized)
	if err != nil {
		// Never fail storage due to payload marshaling; fall back to a compact error record.
		fallback := agentsdk.Event{
			Kind:     agentsdk.EventKindError,
			Protocol: strings.TrimSpace(ev.Protocol),
			Step:     ev.Step,
			Time:     ts,
			Payload:  agentsdk.ErrorEvent{Error: truncateRunes(fmt.Sprintf("event marshal failed: %v", err), 2000)},
		}
		b, _ := json.Marshal(fallback)
		eventJSON = b
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secretary_events (ts, run_id, kind, protocol, step, event_json)
		VALUES (?, ?, ?, ?, ?, ?);
	`, toMillis(ts), rid, string(ev.Kind), strings.TrimSpace(ev.Protocol), ev.Step, string(eventJSON))
	if err != nil {
		return fmt.Errorf("secretary: event append: %w", err)
	}
	return nil
}

// Tail returns the most recent events in chronological order (oldest first).
func (s *EventStore) Tail(ctx context.Context, limit int) ([]StoredEvent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("secretary: event store not initialized")
	}
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, run_id, kind, protocol, step, event_json
		FROM secretary_events
		ORDER BY id DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("secretary: event tail: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []StoredEvent
	for rows.Next() {
		var (
			e        StoredEvent
			tsMillis int64
			kindStr  string
		)
		if err := rows.Scan(&e.ID, &tsMillis, &e.RunID, &kindStr, &e.Protocol, &e.Step, &e.EventJSON); err != nil {
			return nil, fmt.Errorf("secretary: event scan: %w", err)
		}
		e.Time = fromMillis(tsMillis)
		e.Kind = agentsdk.EventKind(kindStr)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("secretary: event rows: %w", err)
	}

	// Reverse so callers see chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *EventStore) PruneKeepLastRuns(ctx context.Context, keepRuns int) error {
	if s == nil || s.db == nil {
		return errors.New("secretary: event store not initialized")
	}
	if keepRuns <= 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM secretary_events;`)
		if err != nil {
			return fmt.Errorf("secretary: event prune: %w", err)
		}
		return nil
	}
	// Keep newest runs based on their latest event id; delete older runs atomically.
	if _, err := s.db.ExecContext(ctx, `
		WITH keep AS (
			SELECT run_id
			FROM secretary_events
			GROUP BY run_id
			ORDER BY MAX(id) DESC
			LIMIT ?
		)
		DELETE FROM secretary_events
		WHERE run_id NOT IN (SELECT run_id FROM keep);
	`, keepRuns); err != nil {
		return fmt.Errorf("secretary: event prune: %w", err)
	}
	return nil
}

func sanitizeEventForStorage(ev agentsdk.Event) agentsdk.Event {
	out := ev
	out.Protocol = strings.TrimSpace(out.Protocol)

	switch out.Kind {
	case agentsdk.EventKindLLMRequest:
		if p, ok := out.Payload.(agentsdk.LLMRequestEvent); ok {
			msgs := make([]agentsdk.Message, 0, len(p.Messages))
			for _, m := range p.Messages {
				msgs = append(msgs, agentsdk.Message{
					Role:    truncateRunes(strings.TrimSpace(m.Role), 40),
					Content: strings.TrimSpace(m.Content),
				})
			}
			p.Messages = msgs
			out.Payload = p
		}
	case agentsdk.EventKindLLMResponse:
		if p, ok := out.Payload.(agentsdk.LLMResponseEvent); ok {
			p.Raw = truncateRunes(strings.TrimSpace(p.Raw), 60_000)
			p.Visible = truncateRunes(strings.TrimSpace(p.Visible), 60_000)
			p.Error = truncateRunes(strings.TrimSpace(p.Error), 4000)
			out.Payload = p
		}
	case agentsdk.EventKindToolCall:
		if p, ok := out.Payload.(agentsdk.ToolCallEvent); ok {
			p.ID = truncateRunes(strings.TrimSpace(p.ID), 200)
			p.Name = truncateRunes(strings.TrimSpace(p.Name), 200)
			if len(p.Fields) > 0 {
				fields := make(map[string]string, len(p.Fields))
				for k, v := range p.Fields {
					fields[truncateRunes(strings.TrimSpace(k), 200)] = truncateRunes(strings.TrimSpace(v), 4000)
				}
				p.Fields = fields
			}
			p.Raw = truncateRunes(strings.TrimSpace(p.Raw), 60_000)
			out.Payload = p
		}
	case agentsdk.EventKindToolResult:
		if p, ok := out.Payload.(agentsdk.ToolResultEvent); ok {
			p.ToolName = truncateRunes(strings.TrimSpace(p.ToolName), 200)
			p.ToolCallID = truncateRunes(strings.TrimSpace(p.ToolCallID), 200)
			p.OutputJSON = truncateRunes(strings.TrimSpace(p.OutputJSON), 60_000)
			p.Error = truncateRunes(strings.TrimSpace(p.Error), 4000)
			out.Payload = p
		}
	case agentsdk.EventKindTrace:
		if p, ok := out.Payload.(agentsdk.TraceEvent); ok {
			p.Message = truncateRunes(strings.TrimSpace(p.Message), 4000)
			out.Payload = p
		}
	case agentsdk.EventKindError:
		if p, ok := out.Payload.(agentsdk.ErrorEvent); ok {
			p.Error = truncateRunes(strings.TrimSpace(p.Error), 4000)
			out.Payload = p
		}
	}

	return out
}

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func fromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}

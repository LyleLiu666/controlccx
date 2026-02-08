package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"controlccx/internal/tasks"
)

const (
	defaultLimit   = 100
	maxLimit       = 500
	maxFetchPerSrc = 2000
)

type Service struct {
	db *sql.DB

	now       func() time.Time
	retention RetentionOptions

	gcMu   sync.RWMutex
	gcLast *GCStatus
}

func NewService(db *sql.DB, opts Options) *Service {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Service{
		db:        db,
		now:       nowFn,
		retention: normalizeRetentionOptions(opts.Retention),
	}
}

func (s *Service) Query(ctx context.Context, in Query) (QueryResult, error) {
	if s == nil || s.db == nil {
		return QueryResult{}, errors.New("audit: service not initialized")
	}
	query, err := normalizeQuery(in)
	if err != nil {
		return QueryResult{}, err
	}

	perSourceLimit := query.Limit * 4
	if perSourceLimit < 200 {
		perSourceLimit = 200
	}
	if perSourceLimit > maxFetchPerSrc {
		perSourceLimit = maxFetchPerSrc
	}

	var merged []Entry
	for _, src := range query.Sources {
		var items []Entry
		switch src {
		case SourceTaskLog:
			if query.RunID != "" {
				continue
			}
			items, err = s.queryTaskLogs(ctx, query, perSourceLimit)
		case SourceTaskTrace:
			if query.RunID != "" {
				continue
			}
			items, err = s.queryTaskTrace(ctx, query, perSourceLimit)
		case SourceSecretaryEvent:
			if query.TaskID != "" {
				continue
			}
			items, err = s.querySecretaryEvents(ctx, query, perSourceLimit)
		case SourceSecretaryCompression:
			if query.TaskID != "" || query.RunID != "" {
				continue
			}
			items, err = s.querySecretaryCompressions(ctx, query, perSourceLimit)
		case SourceSecretaryChat:
			if query.TaskID != "" || query.RunID != "" {
				continue
			}
			items, err = s.querySecretaryChats(ctx, query, perSourceLimit)
		default:
			continue
		}
		if err != nil {
			return QueryResult{}, err
		}
		merged = append(merged, items...)
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Time.Equal(merged[j].Time) {
			return merged[i].ID > merged[j].ID
		}
		return merged[i].Time.After(merged[j].Time)
	})

	if query.cursor.ID != "" {
		filtered := make([]Entry, 0, len(merged))
		for _, item := range merged {
			if isOlderThanCursor(item, query.cursor) {
				filtered = append(filtered, item)
			}
		}
		merged = filtered
	}

	result := QueryResult{Entries: merged}
	if len(result.Entries) > query.Limit {
		result.Entries = result.Entries[:query.Limit]
		result.NextCursor = encodeCursor(result.Entries[len(result.Entries)-1])
	}
	return result, nil
}

func (s *Service) GetEntry(ctx context.Context, entryID string) (EntryDetail, error) {
	if s == nil || s.db == nil {
		return EntryDetail{}, errors.New("audit: service not initialized")
	}
	src, key, err := parseEntryID(entryID)
	if err != nil {
		return EntryDetail{}, err
	}
	switch src {
	case SourceTaskLog:
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return EntryDetail{}, fmt.Errorf("%w: invalid task_log id", ErrEntryNotFound)
		}
		return s.getTaskLogDetail(ctx, id)
	case SourceTaskTrace:
		return s.getTaskTraceDetail(ctx, key)
	case SourceSecretaryEvent:
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return EntryDetail{}, fmt.Errorf("%w: invalid secretary_event id", ErrEntryNotFound)
		}
		return s.getSecretaryEventDetail(ctx, id)
	case SourceSecretaryCompression:
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return EntryDetail{}, fmt.Errorf("%w: invalid secretary_compression id", ErrEntryNotFound)
		}
		return s.getSecretaryCompressionDetail(ctx, id)
	case SourceSecretaryChat:
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return EntryDetail{}, fmt.Errorf("%w: invalid secretary_chat id", ErrEntryNotFound)
		}
		return s.getSecretaryChatDetail(ctx, id)
	default:
		return EntryDetail{}, ErrEntryNotFound
	}
}

func (s *Service) Sources() []SourceInfo {
	return []SourceInfo{
		{
			Source:          SourceTaskLog,
			Label:           "Task Logs",
			DefaultEnabled:  true,
			SupportsTaskID:  true,
			SupportsRunID:   false,
			SupportsStreams: true,
			DefaultStreams: []tasks.LogStream{
				tasks.LogStdout,
				tasks.LogStderr,
				tasks.LogSystem,
				tasks.LogAssistant,
			},
		},
		{
			Source:          SourceTaskTrace,
			Label:           "Task Trace",
			DefaultEnabled:  true,
			SupportsTaskID:  true,
			SupportsRunID:   false,
			SupportsStreams: false,
		},
		{
			Source:          SourceSecretaryEvent,
			Label:           "Secretary Events",
			DefaultEnabled:  true,
			SupportsTaskID:  false,
			SupportsRunID:   true,
			SupportsStreams: false,
		},
		{
			Source:          SourceSecretaryCompression,
			Label:           "Secretary Compression",
			DefaultEnabled:  true,
			SupportsTaskID:  false,
			SupportsRunID:   false,
			SupportsStreams: false,
		},
		{
			Source:          SourceSecretaryChat,
			Label:           "Secretary Chat",
			DefaultEnabled:  true,
			SupportsTaskID:  false,
			SupportsRunID:   false,
			SupportsStreams: false,
		},
	}
}

func (s *Service) Retention() RetentionStatus {
	status := RetentionStatus{
		Days:              s.retention.Days,
		MaxRowsPerSource:  s.retention.MaxRows,
		GCIntervalSeconds: int64(s.retention.GCInterval / time.Second),
	}
	s.gcMu.RLock()
	defer s.gcMu.RUnlock()
	if s.gcLast != nil {
		copied := *s.gcLast
		copied.Results = append([]GCSourceResult(nil), s.gcLast.Results...)
		status.LastRun = &copied
	}
	return status
}

type normalizedQuery struct {
	Sources []Source
	Q       string
	FromMs  int64
	ToMs    int64
	TaskID  string
	RunID   string
	Streams []tasks.LogStream
	Limit   int
	cursor  pageCursor
}

func normalizeQuery(in Query) (normalizedQuery, error) {
	out := normalizedQuery{
		Q:      strings.TrimSpace(in.Q),
		TaskID: strings.TrimSpace(in.TaskID),
		RunID:  strings.TrimSpace(in.RunID),
		Limit:  in.Limit,
	}
	if out.Limit <= 0 {
		out.Limit = defaultLimit
	}
	if out.Limit > maxLimit {
		out.Limit = maxLimit
	}
	if !in.From.IsZero() {
		out.FromMs = toMillis(in.From.UTC())
	}
	if !in.To.IsZero() {
		out.ToMs = toMillis(in.To.UTC())
	}
	if out.FromMs > 0 && out.ToMs > 0 && out.FromMs > out.ToMs {
		return normalizedQuery{}, errors.New("audit: invalid time range")
	}
	if strings.TrimSpace(in.Cursor) != "" {
		cur, err := decodeCursor(in.Cursor)
		if err != nil {
			return normalizedQuery{}, err
		}
		out.cursor = cur
	}

	if len(in.Sources) == 0 {
		out.Sources = append([]Source(nil), allSources...)
	} else {
		seen := make(map[Source]bool, len(in.Sources))
		for _, src := range in.Sources {
			if !isKnownSource(src) {
				return normalizedQuery{}, ErrInvalidSource
			}
			if seen[src] {
				continue
			}
			seen[src] = true
			out.Sources = append(out.Sources, src)
		}
	}

	if len(in.Streams) > 0 {
		seen := map[tasks.LogStream]bool{}
		for _, stream := range in.Streams {
			if strings.TrimSpace(string(stream)) == "" {
				continue
			}
			if seen[stream] {
				continue
			}
			seen[stream] = true
			out.Streams = append(out.Streams, stream)
		}
	}
	return out, nil
}

func (s *Service) queryTaskLogs(ctx context.Context, query normalizedQuery, limit int) ([]Entry, error) {
	var sb strings.Builder
	var args []any
	sb.WriteString(`
		SELECT id, task_id, ts, stream, message
		FROM logs
		WHERE 1=1
	`)
	if query.TaskID != "" {
		sb.WriteString(` AND task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.FromMs > 0 {
		sb.WriteString(` AND ts >= ?`)
		args = append(args, query.FromMs)
	}
	if query.ToMs > 0 {
		sb.WriteString(` AND ts <= ?`)
		args = append(args, query.ToMs)
	}
	if len(query.Streams) > 0 {
		sb.WriteString(` AND stream IN (`)
		for i, stream := range query.Streams {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("?")
			args = append(args, string(stream))
		}
		sb.WriteString(`)`)
	}
	if query.Q != "" {
		sb.WriteString(` AND instr(lower(message), lower(?)) > 0`)
		args = append(args, query.Q)
	}
	sb.WriteString(` ORDER BY ts DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query task_log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			id      int64
			taskID  string
			ts      int64
			stream  string
			message string
		)
		if err := rows.Scan(&id, &taskID, &ts, &stream, &message); err != nil {
			return nil, fmt.Errorf("audit: scan task_log: %w", err)
		}
		msg := strings.TrimSpace(message)
		out = append(out, Entry{
			ID:         fmt.Sprintf("%s:%d", SourceTaskLog, id),
			Source:     SourceTaskLog,
			Time:       fromMillis(ts),
			TaskID:     strings.TrimSpace(taskID),
			Title:      strings.TrimSpace("Task log · " + stream),
			Summary:    truncateRunes(msg, 240),
			RawPreview: truncateRunes(msg, 1200),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows task_log: %w", err)
	}
	return out, nil
}

func (s *Service) queryTaskTrace(ctx context.Context, query normalizedQuery, limit int) ([]Entry, error) {
	var sb strings.Builder
	var args []any
	sb.WriteString(`
		SELECT task_id, cmd, args_json, dir, env_keys_json, created_at
		FROM task_invocations
		WHERE 1=1
	`)
	if query.TaskID != "" {
		sb.WriteString(` AND task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.FromMs > 0 {
		sb.WriteString(` AND created_at >= ?`)
		args = append(args, query.FromMs)
	}
	if query.ToMs > 0 {
		sb.WriteString(` AND created_at <= ?`)
		args = append(args, query.ToMs)
	}
	if query.Q != "" {
		sb.WriteString(` AND (
			instr(lower(task_id), lower(?)) > 0 OR
			instr(lower(cmd), lower(?)) > 0 OR
			instr(lower(args_json), lower(?)) > 0 OR
			instr(lower(dir), lower(?)) > 0
		)`)
		args = append(args, query.Q, query.Q, query.Q, query.Q)
	}
	sb.WriteString(` ORDER BY created_at DESC, task_id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query task_trace: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			taskID    string
			cmd       string
			argsJSON  string
			dir       string
			envJSON   string
			createdAt int64
		)
		if err := rows.Scan(&taskID, &cmd, &argsJSON, &dir, &envJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("audit: scan task_trace: %w", err)
		}
		args := parseStringArray(argsJSON)
		summary := strings.TrimSpace(cmd + " " + strings.Join(args, " "))
		raw := strings.TrimSpace(fmt.Sprintf("cmd=%s\ndir=%s\nargs=%s\nenv_keys=%s", cmd, dir, argsJSON, envJSON))
		out = append(out, Entry{
			ID:         fmt.Sprintf("%s:%s", SourceTaskTrace, strings.TrimSpace(taskID)),
			Source:     SourceTaskTrace,
			Time:       fromMillis(createdAt),
			TaskID:     strings.TrimSpace(taskID),
			Title:      strings.TrimSpace("Task trace · " + strings.TrimSpace(cmd)),
			Summary:    truncateRunes(summary, 240),
			RawPreview: truncateRunes(raw, 1200),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows task_trace: %w", err)
	}
	return out, nil
}

func (s *Service) querySecretaryEvents(ctx context.Context, query normalizedQuery, limit int) ([]Entry, error) {
	var sb strings.Builder
	var args []any
	sb.WriteString(`
		SELECT id, ts, run_id, kind, protocol, step, event_json
		FROM secretary_events
		WHERE 1=1
	`)
	if query.RunID != "" {
		sb.WriteString(` AND run_id = ?`)
		args = append(args, query.RunID)
	}
	if query.FromMs > 0 {
		sb.WriteString(` AND ts >= ?`)
		args = append(args, query.FromMs)
	}
	if query.ToMs > 0 {
		sb.WriteString(` AND ts <= ?`)
		args = append(args, query.ToMs)
	}
	if query.Q != "" {
		sb.WriteString(` AND (
			instr(lower(run_id), lower(?)) > 0 OR
			instr(lower(kind), lower(?)) > 0 OR
			instr(lower(protocol), lower(?)) > 0 OR
			instr(lower(event_json), lower(?)) > 0
		)`)
		args = append(args, query.Q, query.Q, query.Q, query.Q)
	}
	sb.WriteString(` ORDER BY ts DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query secretary_event: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			id       int64
			ts       int64
			runID    string
			kind     string
			protocol string
			step     int
			eventRaw string
		)
		if err := rows.Scan(&id, &ts, &runID, &kind, &protocol, &step, &eventRaw); err != nil {
			return nil, fmt.Errorf("audit: scan secretary_event: %w", err)
		}
		summary := strings.TrimSpace(fmt.Sprintf("run=%s protocol=%s step=%d", runID, protocol, step))
		out = append(out, Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryEvent, id),
			Source:     SourceSecretaryEvent,
			Time:       fromMillis(ts),
			RunID:      strings.TrimSpace(runID),
			Title:      strings.TrimSpace("Secretary event · " + strings.TrimSpace(kind)),
			Summary:    truncateRunes(summary, 240),
			RawPreview: truncateRunes(strings.TrimSpace(eventRaw), 1200),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows secretary_event: %w", err)
	}
	return out, nil
}

func (s *Service) querySecretaryCompressions(ctx context.Context, query normalizedQuery, limit int) ([]Entry, error) {
	var sb strings.Builder
	var args []any
	sb.WriteString(`
		SELECT id, ts, cursor_before, cursor_after, keep_from, summary, backend, error
		FROM secretary_compressions
		WHERE 1=1
	`)
	if query.FromMs > 0 {
		sb.WriteString(` AND ts >= ?`)
		args = append(args, query.FromMs)
	}
	if query.ToMs > 0 {
		sb.WriteString(` AND ts <= ?`)
		args = append(args, query.ToMs)
	}
	if query.Q != "" {
		sb.WriteString(` AND (
			instr(lower(summary), lower(?)) > 0 OR
			instr(lower(backend), lower(?)) > 0 OR
			instr(lower(error), lower(?)) > 0
		)`)
		args = append(args, query.Q, query.Q, query.Q)
	}
	sb.WriteString(` ORDER BY ts DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query secretary_compression: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			id           int64
			ts           int64
			cursorBefore int64
			cursorAfter  int64
			keepFrom     int64
			summary      string
			backend      string
			errText      string
		)
		if err := rows.Scan(&id, &ts, &cursorBefore, &cursorAfter, &keepFrom, &summary, &backend, &errText); err != nil {
			return nil, fmt.Errorf("audit: scan secretary_compression: %w", err)
		}
		metaRaw := strings.TrimSpace(fmt.Sprintf(
			"backend=%s cursor_before=%d cursor_after=%d keep_from=%d\nsummary=%s\nerror=%s",
			strings.TrimSpace(backend), cursorBefore, cursorAfter, keepFrom, strings.TrimSpace(summary), strings.TrimSpace(errText),
		))
		displaySummary := strings.TrimSpace(summary)
		if displaySummary == "" {
			displaySummary = strings.TrimSpace(errText)
		}
		out = append(out, Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryCompression, id),
			Source:     SourceSecretaryCompression,
			Time:       fromMillis(ts),
			Title:      "Secretary compression",
			Summary:    truncateRunes(displaySummary, 240),
			RawPreview: truncateRunes(metaRaw, 1200),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows secretary_compression: %w", err)
	}
	return out, nil
}

func (s *Service) querySecretaryChats(ctx context.Context, query normalizedQuery, limit int) ([]Entry, error) {
	var sb strings.Builder
	var args []any
	sb.WriteString(`
		SELECT id, ts, role, content
		FROM chat_messages
		WHERE 1=1
	`)
	if query.FromMs > 0 {
		sb.WriteString(` AND ts >= ?`)
		args = append(args, query.FromMs)
	}
	if query.ToMs > 0 {
		sb.WriteString(` AND ts <= ?`)
		args = append(args, query.ToMs)
	}
	if query.Q != "" {
		sb.WriteString(` AND (
			instr(lower(role), lower(?)) > 0 OR
			instr(lower(content), lower(?)) > 0
		)`)
		args = append(args, query.Q, query.Q)
	}
	sb.WriteString(` ORDER BY ts DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query secretary_chat: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			id      int64
			ts      int64
			role    string
			content string
		)
		if err := rows.Scan(&id, &ts, &role, &content); err != nil {
			return nil, fmt.Errorf("audit: scan secretary_chat: %w", err)
		}
		text := strings.TrimSpace(content)
		out = append(out, Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryChat, id),
			Source:     SourceSecretaryChat,
			Time:       fromMillis(ts),
			Title:      strings.TrimSpace("Secretary chat · " + strings.TrimSpace(role)),
			Summary:    truncateRunes(text, 240),
			RawPreview: truncateRunes(text, 1200),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows secretary_chat: %w", err)
	}
	return out, nil
}

func parseEntryID(entryID string) (Source, string, error) {
	raw := strings.TrimSpace(entryID)
	if raw == "" {
		return "", "", ErrEntryNotFound
	}
	srcRaw, key, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(srcRaw) == "" || strings.TrimSpace(key) == "" {
		return "", "", ErrEntryNotFound
	}
	src := Source(strings.TrimSpace(srcRaw))
	if !isKnownSource(src) {
		return "", "", ErrEntryNotFound
	}
	return src, strings.TrimSpace(key), nil
}

func (s *Service) getTaskLogDetail(ctx context.Context, id int64) (EntryDetail, error) {
	var (
		taskID  string
		ts      int64
		stream  string
		message string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT task_id, ts, stream, message
		FROM logs
		WHERE id = ?;
	`, id).Scan(&taskID, &ts, &stream, &message)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryDetail{}, ErrEntryNotFound
		}
		return EntryDetail{}, fmt.Errorf("audit: get task_log detail: %w", err)
	}
	msg := strings.TrimSpace(message)
	return EntryDetail{
		Entry: Entry{
			ID:         fmt.Sprintf("%s:%d", SourceTaskLog, id),
			Source:     SourceTaskLog,
			Time:       fromMillis(ts),
			TaskID:     strings.TrimSpace(taskID),
			Title:      strings.TrimSpace("Task log · " + stream),
			Summary:    truncateRunes(msg, 240),
			RawPreview: truncateRunes(msg, 1200),
		},
		Raw: msg,
		Meta: map[string]any{
			"stream": stream,
		},
	}, nil
}

func (s *Service) getTaskTraceDetail(ctx context.Context, taskID string) (EntryDetail, error) {
	var (
		cmd       string
		argsJSON  string
		dir       string
		envJSON   string
		createdAt int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT cmd, args_json, dir, env_keys_json, created_at
		FROM task_invocations
		WHERE task_id = ?;
	`, taskID).Scan(&cmd, &argsJSON, &dir, &envJSON, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryDetail{}, ErrEntryNotFound
		}
		return EntryDetail{}, fmt.Errorf("audit: get task_trace detail: %w", err)
	}
	args := parseStringArray(argsJSON)
	envKeys := parseStringArray(envJSON)
	summary := strings.TrimSpace(cmd + " " + strings.Join(args, " "))
	raw := marshalRaw(map[string]any{
		"task_id":           taskID,
		"cmd":               cmd,
		"args":              args,
		"dir":               dir,
		"env_injected_keys": envKeys,
		"created_at":        fromMillis(createdAt).Format(time.RFC3339Nano),
	})
	return EntryDetail{
		Entry: Entry{
			ID:         fmt.Sprintf("%s:%s", SourceTaskTrace, taskID),
			Source:     SourceTaskTrace,
			Time:       fromMillis(createdAt),
			TaskID:     strings.TrimSpace(taskID),
			Title:      strings.TrimSpace("Task trace · " + strings.TrimSpace(cmd)),
			Summary:    truncateRunes(summary, 240),
			RawPreview: truncateRunes(raw, 1200),
		},
		Raw: raw,
		Meta: map[string]any{
			"cmd":               cmd,
			"args":              args,
			"dir":               dir,
			"env_injected_keys": envKeys,
		},
	}, nil
}

func (s *Service) getSecretaryEventDetail(ctx context.Context, id int64) (EntryDetail, error) {
	var (
		ts       int64
		runID    string
		kind     string
		protocol string
		step     int
		eventRaw string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT ts, run_id, kind, protocol, step, event_json
		FROM secretary_events
		WHERE id = ?;
	`, id).Scan(&ts, &runID, &kind, &protocol, &step, &eventRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryDetail{}, ErrEntryNotFound
		}
		return EntryDetail{}, fmt.Errorf("audit: get secretary_event detail: %w", err)
	}
	summary := strings.TrimSpace(fmt.Sprintf("run=%s protocol=%s step=%d", runID, protocol, step))
	return EntryDetail{
		Entry: Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryEvent, id),
			Source:     SourceSecretaryEvent,
			Time:       fromMillis(ts),
			RunID:      strings.TrimSpace(runID),
			Title:      strings.TrimSpace("Secretary event · " + strings.TrimSpace(kind)),
			Summary:    truncateRunes(summary, 240),
			RawPreview: truncateRunes(strings.TrimSpace(eventRaw), 1200),
		},
		Raw: strings.TrimSpace(eventRaw),
		Meta: map[string]any{
			"kind":     kind,
			"protocol": protocol,
			"step":     step,
		},
	}, nil
}

func (s *Service) getSecretaryCompressionDetail(ctx context.Context, id int64) (EntryDetail, error) {
	var (
		ts           int64
		cursorBefore int64
		cursorAfter  int64
		keepFrom     int64
		summary      string
		backend      string
		errText      string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT ts, cursor_before, cursor_after, keep_from, summary, backend, error
		FROM secretary_compressions
		WHERE id = ?;
	`, id).Scan(&ts, &cursorBefore, &cursorAfter, &keepFrom, &summary, &backend, &errText)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryDetail{}, ErrEntryNotFound
		}
		return EntryDetail{}, fmt.Errorf("audit: get secretary_compression detail: %w", err)
	}
	displaySummary := strings.TrimSpace(summary)
	if displaySummary == "" {
		displaySummary = strings.TrimSpace(errText)
	}
	raw := marshalRaw(map[string]any{
		"cursor_before": cursorBefore,
		"cursor_after":  cursorAfter,
		"keep_from":     keepFrom,
		"summary":       strings.TrimSpace(summary),
		"backend":       strings.TrimSpace(backend),
		"error":         strings.TrimSpace(errText),
	})
	return EntryDetail{
		Entry: Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryCompression, id),
			Source:     SourceSecretaryCompression,
			Time:       fromMillis(ts),
			Title:      "Secretary compression",
			Summary:    truncateRunes(displaySummary, 240),
			RawPreview: truncateRunes(raw, 1200),
		},
		Raw: raw,
		Meta: map[string]any{
			"cursor_before": cursorBefore,
			"cursor_after":  cursorAfter,
			"keep_from":     keepFrom,
			"backend":       strings.TrimSpace(backend),
		},
	}, nil
}

func (s *Service) getSecretaryChatDetail(ctx context.Context, id int64) (EntryDetail, error) {
	var (
		ts      int64
		role    string
		content string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT ts, role, content
		FROM chat_messages
		WHERE id = ?;
	`, id).Scan(&ts, &role, &content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryDetail{}, ErrEntryNotFound
		}
		return EntryDetail{}, fmt.Errorf("audit: get secretary_chat detail: %w", err)
	}
	text := strings.TrimSpace(content)
	meta := map[string]any{
		"role": role,
	}
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		telemetry, err := s.secretaryChatTelemetry(ctx, ts)
		if err == nil {
			if strings.TrimSpace(telemetry.RunID) != "" {
				meta["run_id"] = strings.TrimSpace(telemetry.RunID)
			}
			if len(telemetry.KVCache) > 0 {
				meta["kv_cache"] = telemetry.KVCache
			}
			if len(telemetry.ProviderReceipt) > 0 {
				meta["provider_receipt"] = telemetry.ProviderReceipt
			}
		}
	}
	return EntryDetail{
		Entry: Entry{
			ID:         fmt.Sprintf("%s:%d", SourceSecretaryChat, id),
			Source:     SourceSecretaryChat,
			Time:       fromMillis(ts),
			Title:      strings.TrimSpace("Secretary chat · " + strings.TrimSpace(role)),
			Summary:    truncateRunes(text, 240),
			RawPreview: truncateRunes(text, 1200),
		},
		Raw:  text,
		Meta: meta,
	}, nil
}

type secretaryChatTelemetryMeta struct {
	RunID           string
	KVCache         map[string]any
	ProviderReceipt map[string]any
}

func (s *Service) secretaryChatTelemetry(ctx context.Context, chatTs int64) (secretaryChatTelemetryMeta, error) {
	if s == nil || s.db == nil {
		return secretaryChatTelemetryMeta{}, errors.New("audit: service not initialized")
	}
	if chatTs <= 0 {
		return secretaryChatTelemetryMeta{}, nil
	}

	var runID string
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id
		FROM secretary_events
		WHERE ts <= ?
		ORDER BY ts DESC, id DESC
		LIMIT 1;
	`, chatTs).Scan(&runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return secretaryChatTelemetryMeta{}, nil
		}
		return secretaryChatTelemetryMeta{}, fmt.Errorf("audit: secretary chat telemetry run: %w", err)
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return secretaryChatTelemetryMeta{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, event_json
		FROM secretary_events
		WHERE run_id = ?
		ORDER BY id ASC;
	`, runID)
	if err != nil {
		return secretaryChatTelemetryMeta{}, fmt.Errorf("audit: secretary chat telemetry events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := secretaryChatTelemetryMeta{RunID: runID}

	for rows.Next() {
		var (
			kind     string
			eventRaw string
		)
		if err := rows.Scan(&kind, &eventRaw); err != nil {
			return secretaryChatTelemetryMeta{}, fmt.Errorf("audit: secretary chat telemetry scan: %w", err)
		}

		parsedKind, payload := parseSecretaryEventPayload(kind, eventRaw)
		switch parsedKind {
		case "llm_request":
			if reqKV := extractKVCacheFromLLMRequestPayload(payload); len(reqKV) > 0 && len(out.KVCache) == 0 {
				out.KVCache = reqKV
			}
		case "provider_receipt":
			if len(payload) > 0 {
				out.ProviderReceipt = cloneMapAny(payload)
			}
			if kv := extractKVCacheFromProviderReceipt(payload); len(kv) > 0 {
				out.KVCache = kv
			}
		}
	}
	if err := rows.Err(); err != nil {
		return secretaryChatTelemetryMeta{}, fmt.Errorf("audit: secretary chat telemetry rows: %w", err)
	}
	return out, nil
}

func parseSecretaryEventPayload(kind string, eventRaw string) (string, map[string]any) {
	rowKind := strings.ToLower(strings.TrimSpace(kind))
	rawText := strings.TrimSpace(eventRaw)
	if rawText == "" {
		return rowKind, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(rawText), &root); err != nil || len(root) == 0 {
		return rowKind, nil
	}

	kindVal := strings.TrimSpace(anyStringValue(root["kind"]))
	if kindVal == "" {
		kindVal = strings.TrimSpace(anyStringValue(root["Kind"]))
	}
	if kindVal != "" {
		rowKind = strings.ToLower(kindVal)
	}

	payload := anyMapValue(root["payload"])
	if payload == nil {
		payload = anyMapValue(root["Payload"])
	}
	if payload == nil {
		payload = cloneMapAny(root)
	}
	return rowKind, payload
}

func extractKVCacheFromLLMRequestPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	opts := anyMapValue(payload["options"])
	if opts == nil {
		opts = anyMapValue(payload["Options"])
	}
	if len(opts) == 0 {
		return nil
	}
	out := map[string]any{}
	if v, ok := anyBoolValue(opts["enable_prompt_cache"]); ok {
		out["request_prompt_cache_enabled"] = v
	} else if v, ok := anyBoolValue(opts["EnablePromptCache"]); ok {
		out["request_prompt_cache_enabled"] = v
	}

	if v, ok := anyIntValue(opts["cache_epoch"]); ok {
		out["request_cache_epoch"] = v
	} else if v, ok := anyIntValue(opts["CacheEpoch"]); ok {
		out["request_cache_epoch"] = v
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func extractKVCacheFromProviderReceipt(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := map[string]any{}

	if kv := anyMapValue(payload["kv_cache"]); len(kv) > 0 {
		for k, v := range kv {
			out[k] = v
		}
	}

	if usage := anyMapValue(payload["usage"]); len(usage) > 0 {
		for _, key := range []string{"cache_read_input_tokens", "cache_creation_input_tokens", "cached_input_tokens"} {
			if n, ok := anyIntValue(usage[key]); ok {
				out[key] = n
			}
		}
		if promptDetails := anyMapValue(usage["prompt_tokens_details"]); len(promptDetails) > 0 {
			if n, ok := anyIntValue(promptDetails["cached_tokens"]); ok {
				out["prompt_cached_tokens"] = n
			}
		}
	}

	for _, key := range []string{"request_prompt_cache_enabled", "request_cache_epoch", "request_cache_marked_blocks"} {
		if v, ok := payload[key]; ok {
			out[key] = v
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func anyMapValue(v any) map[string]any {
	m, _ := v.(map[string]any)
	if len(m) == 0 {
		return nil
	}
	return m
}

func anyStringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func anyBoolValue(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	default:
		return false, false
	}
}

func anyIntValue(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int64(x), true
	case float32:
		return int64(x), true
	case float64:
		return int64(x), true
	default:
		return 0, false
	}
}

func cloneMapAny(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[k] = cloneMapAny(x)
		case []any:
			out[k] = cloneSliceAny(x)
		default:
			out[k] = x
		}
	}
	return out
}

func cloneSliceAny(in []any) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, 0, len(in))
	for _, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out = append(out, cloneMapAny(x))
		case []any:
			out = append(out, cloneSliceAny(x))
		default:
			out = append(out, x)
		}
	}
	return out
}

func parseStringArray(raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func marshalRaw(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

func truncateRunes(text string, max int) string {
	value := strings.TrimSpace(text)
	if max <= 0 {
		return value
	}
	count := 0
	for range value {
		count++
		if count > max {
			break
		}
	}
	if count <= max {
		return value
	}
	var builder strings.Builder
	builder.Grow(max * 3)
	n := 0
	for _, r := range value {
		if n >= max-1 {
			break
		}
		builder.WriteRune(r)
		n++
	}
	builder.WriteRune('…')
	return builder.String()
}

package audit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestService_QueryFiltersSortAndCursor(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	base := time.Date(2026, 2, 8, 8, 0, 0, 0, time.UTC)
	mustInsertTask(t, ctx, conn, "task-1", base)
	mustInsertTask(t, ctx, conn, "task-2", base)
	mustExecDB(t, ctx, conn, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-1", toMs(base.Add(1*time.Minute)), "stdout", "alpha output")
	mustExecDB(t, ctx, conn, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-1", toMs(base.Add(2*time.Minute)), "system", "approval blocked")
	mustExecDB(t, ctx, conn, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-2", toMs(base.Add(3*time.Minute)), "stderr", "bravo fail")

	mustExecDB(t, ctx, conn, `INSERT INTO task_invocations(task_id, cmd, args_json, dir, env_keys_json, created_at) VALUES (?, ?, ?, ?, ?, ?);`, "task-1", "claude", `["run","alpha"]`, "/tmp/task-1", `["ANTHROPIC_API_KEY"]`, toMs(base.Add(4*time.Minute)))
	mustExecDB(t, ctx, conn, `INSERT INTO task_invocations(task_id, cmd, args_json, dir, env_keys_json, created_at) VALUES (?, ?, ?, ?, ?, ?);`, "task-2", "codex", `["run","bravo"]`, "/tmp/task-2", `["OPENAI_API_KEY"]`, toMs(base.Add(5*time.Minute)))

	mustExecDB(t, ctx, conn, `INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`, toMs(base.Add(6*time.Minute)), "run-1", "tool_call", "xml", 1, `{"name":"tasks_count","query":"alpha"}`)
	mustExecDB(t, ctx, conn, `INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`, toMs(base.Add(7*time.Minute)), "run-2", "error", "xml", 2, `{"error":"failed"}`)

	mustExecDB(t, ctx, conn, `INSERT INTO secretary_compressions(ts, cursor_before, cursor_after, keep_from, summary, backend, error) VALUES (?, ?, ?, ?, ?, ?, ?);`, toMs(base.Add(8*time.Minute)), 10, 20, 21, "alpha summary", "simple-http", "")
	mustExecDB(t, ctx, conn, `INSERT INTO secretary_compressions(ts, cursor_before, cursor_after, keep_from, summary, backend, error) VALUES (?, ?, ?, ?, ?, ?, ?);`, toMs(base.Add(9*time.Minute)), 20, 30, 31, "", "simple-http", "compress failed")

	mustExecDB(t, ctx, conn, `INSERT INTO chat_messages(ts, role, content) VALUES (?, ?, ?);`, toMs(base.Add(10*time.Minute)), "user", "你好 alpha")
	mustExecDB(t, ctx, conn, `INSERT INTO chat_messages(ts, role, content) VALUES (?, ?, ?);`, toMs(base.Add(11*time.Minute)), "assistant", "当前失败 1 个")

	svc := NewService(conn, Options{})

	all, err := svc.Query(ctx, Query{Limit: 100})
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(all.Entries) < 8 {
		t.Fatalf("entries len=%d want >=8", len(all.Entries))
	}
	assertDescending(t, all.Entries)

	logsOnly, err := svc.Query(ctx, Query{
		Sources: []Source{SourceTaskLog},
		Streams: []tasks.LogStream{tasks.LogSystem},
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("query logsOnly: %v", err)
	}
	if len(logsOnly.Entries) != 1 {
		t.Fatalf("logsOnly len=%d want 1", len(logsOnly.Entries))
	}
	if logsOnly.Entries[0].Source != SourceTaskLog {
		t.Fatalf("source=%s want %s", logsOnly.Entries[0].Source, SourceTaskLog)
	}
	if logsOnly.Entries[0].TaskID != "task-1" {
		t.Fatalf("task_id=%q want task-1", logsOnly.Entries[0].TaskID)
	}

	byTask, err := svc.Query(ctx, Query{TaskID: "task-1", Limit: 50})
	if err != nil {
		t.Fatalf("query byTask: %v", err)
	}
	for _, item := range byTask.Entries {
		if item.TaskID != "task-1" {
			t.Fatalf("unexpected task_id in task-filtered query: %+v", item)
		}
	}

	byRun, err := svc.Query(ctx, Query{RunID: "run-1", Limit: 50})
	if err != nil {
		t.Fatalf("query byRun: %v", err)
	}
	if len(byRun.Entries) != 1 {
		t.Fatalf("byRun len=%d want 1", len(byRun.Entries))
	}
	if byRun.Entries[0].Source != SourceSecretaryEvent || byRun.Entries[0].RunID != "run-1" {
		t.Fatalf("unexpected byRun entry: %+v", byRun.Entries[0])
	}

	byKeyword, err := svc.Query(ctx, Query{Q: "alpha", Limit: 100})
	if err != nil {
		t.Fatalf("query byKeyword: %v", err)
	}
	if len(byKeyword.Entries) == 0 {
		t.Fatalf("expected keyword results")
	}
	for _, item := range byKeyword.Entries {
		hay := strings.ToLower(item.Title + "\n" + item.Summary + "\n" + item.RawPreview)
		if !strings.Contains(hay, "alpha") {
			t.Fatalf("keyword filter leaked non-matching entry: %+v", item)
		}
	}

	ranged, err := svc.Query(ctx, Query{
		From:  base.Add(8 * time.Minute),
		To:    base.Add(10 * time.Minute),
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("query ranged: %v", err)
	}
	for _, item := range ranged.Entries {
		if item.Time.Before(base.Add(8*time.Minute)) || item.Time.After(base.Add(10*time.Minute)) {
			t.Fatalf("time out of range: %s", item.Time)
		}
	}

	page1, err := svc.Query(ctx, Query{Limit: 3})
	if err != nil {
		t.Fatalf("query page1: %v", err)
	}
	if len(page1.Entries) != 3 {
		t.Fatalf("page1 len=%d want 3", len(page1.Entries))
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected next cursor")
	}

	page2, err := svc.Query(ctx, Query{Limit: 3, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("query page2: %v", err)
	}
	if len(page2.Entries) == 0 {
		t.Fatalf("expected non-empty page2")
	}
	assertDescending(t, page2.Entries)
	for _, p1 := range page1.Entries {
		for _, p2 := range page2.Entries {
			if p1.ID == p2.ID {
				t.Fatalf("duplicate entry across pages: %s", p1.ID)
			}
		}
	}
}

func TestService_GetEntryDetail(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	nowMs := toMs(time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC))
	mustExecDB(t, ctx, conn, `INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`, nowMs, "run-x", "trace", "xml", 3, `{"k":"v"}`)
	svc := NewService(conn, Options{})

	list, err := svc.Query(ctx, Query{Sources: []Source{SourceSecretaryEvent}, Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(list.Entries) != 1 {
		t.Fatalf("entries len=%d want 1", len(list.Entries))
	}

	detail, err := svc.GetEntry(ctx, list.Entries[0].ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Source != SourceSecretaryEvent {
		t.Fatalf("source=%s want %s", detail.Source, SourceSecretaryEvent)
	}
	if detail.RunID != "run-x" {
		t.Fatalf("run_id=%q want run-x", detail.RunID)
	}
	if detail.Raw == "" {
		t.Fatalf("expected raw")
	}
}

func TestService_GetSecretaryChatDetail_EnrichesKVCacheAndProviderReceipt(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	base := time.Date(2026, 2, 8, 22, 0, 0, 0, time.UTC)
	mustExecDB(
		t, ctx, conn,
		`INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`,
		toMs(base.Add(1*time.Second)),
		"run-kv",
		"llm_request",
		"xml",
		0,
		`{"kind":"llm_request","payload":{"options":{"EnablePromptCache":true,"CacheEpoch":12}}}`,
	)
	mustExecDB(
		t, ctx, conn,
		`INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`,
		toMs(base.Add(2*time.Second)),
		"run-kv",
		"provider_receipt",
		"http",
		0,
		`{"kind":"provider_receipt","payload":{"provider":"anthropic","request_id":"req-xyz","usage":{"input_tokens":120,"output_tokens":30,"cache_read_input_tokens":90},"kv_cache":{"cache_read_input_tokens":90}}}`,
	)
	mustExecDB(t, ctx, conn, `INSERT INTO chat_messages(ts, role, content) VALUES (?, ?, ?);`, toMs(base.Add(3*time.Second)), "user", "查一下任务")
	mustExecDB(t, ctx, conn, `INSERT INTO chat_messages(ts, role, content) VALUES (?, ?, ?);`, toMs(base.Add(4*time.Second)), "assistant", "好的")

	svc := NewService(conn, Options{})
	list, err := svc.Query(ctx, Query{
		Sources: []Source{SourceSecretaryChat},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("query chat list: %v", err)
	}
	if len(list.Entries) == 0 {
		t.Fatalf("expected secretary chat entries")
	}

	var assistantID string
	for _, item := range list.Entries {
		if strings.Contains(strings.ToLower(item.Title), "assistant") {
			assistantID = item.ID
			break
		}
	}
	if strings.TrimSpace(assistantID) == "" {
		t.Fatalf("expected assistant entry in secretary chat list")
	}

	detail, err := svc.GetEntry(ctx, assistantID)
	if err != nil {
		t.Fatalf("get assistant detail: %v", err)
	}
	if detail.Source != SourceSecretaryChat {
		t.Fatalf("source=%s want %s", detail.Source, SourceSecretaryChat)
	}
	if detail.Meta == nil {
		t.Fatalf("expected meta in secretary chat detail")
	}
	if got := strings.TrimSpace(anyString(detail.Meta["run_id"])); got != "run-kv" {
		t.Fatalf("meta.run_id=%q want %q", got, "run-kv")
	}

	kv, _ := detail.Meta["kv_cache"].(map[string]any)
	if kv == nil {
		t.Fatalf("expected kv_cache map in meta, got=%T", detail.Meta["kv_cache"])
	}
	if got := int(anyFloat(kv["cache_read_input_tokens"])); got != 90 {
		t.Fatalf("kv.cache_read_input_tokens=%d want 90", got)
	}

	receipt, _ := detail.Meta["provider_receipt"].(map[string]any)
	if receipt == nil {
		t.Fatalf("expected provider_receipt map in meta, got=%T", detail.Meta["provider_receipt"])
	}
	if got := strings.TrimSpace(anyString(receipt["request_id"])); got != "req-xyz" {
		t.Fatalf("receipt.request_id=%q want req-xyz", got)
	}
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func anyFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func assertDescending(t *testing.T, entries []Entry) {
	t.Helper()
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		cur := entries[i]
		if prev.Time.Before(cur.Time) {
			t.Fatalf("entries not descending at %d: %s < %s", i, prev.Time, cur.Time)
		}
		if prev.Time.Equal(cur.Time) && prev.ID < cur.ID {
			t.Fatalf("entries tie-break not descending at %d: %s < %s", i, prev.ID, cur.ID)
		}
	}
}

package audit

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestService_RunGCAppliesAgeAndCountLimits(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	now := time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	mustInsertTask(t, ctx, conn, "task-ret", old)

	for i := range 4 {
		ts := now.Add(time.Duration(i) * time.Minute)
		if i == 0 {
			ts = old
		}
		mustExecDB(t, ctx, conn, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-ret", toMs(ts), "system", "retention-log")
		invokeTaskID := "task-ret-" + string(rune('A'+i))
		mustInsertTask(t, ctx, conn, invokeTaskID, ts)
		mustExecDB(t, ctx, conn, `INSERT OR REPLACE INTO task_invocations(task_id, cmd, args_json, dir, env_keys_json, created_at) VALUES (?, ?, ?, ?, ?, ?);`,
			invokeTaskID, "claude", `[]`, "/tmp", `[]`, toMs(ts))
		mustExecDB(t, ctx, conn, `INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`, toMs(ts), "run-ret", "trace", "xml", i, `{"i":1}`)
		mustExecDB(t, ctx, conn, `INSERT INTO secretary_compressions(ts, cursor_before, cursor_after, keep_from, summary, backend, error) VALUES (?, ?, ?, ?, ?, ?, ?);`, toMs(ts), i, i+1, i+2, "sum", "simple-http", "")
		mustExecDB(t, ctx, conn, `INSERT INTO chat_messages(ts, role, content) VALUES (?, ?, ?);`, toMs(ts), "assistant", "retention-chat")
	}

	svc := NewService(conn, Options{
		Now: func() time.Time { return now },
		Retention: RetentionOptions{
			Days:          1,
			MaxRows:       2,
			GCInterval:    time.Hour,
			StartupRunGC:  false,
			PreviewRunCap: 0,
		},
	})

	status := svc.RunGC(ctx)
	if status.RunAt.IsZero() {
		t.Fatalf("expected run_at")
	}
	if len(status.Results) == 0 {
		t.Fatalf("expected source results")
	}

	assertCountLE(t, ctx, conn, "logs", 2)
	assertCountLE(t, ctx, conn, "task_invocations", 2)
	assertCountLE(t, ctx, conn, "secretary_events", 2)
	assertCountLE(t, ctx, conn, "secretary_compressions", 2)
	assertCountLE(t, ctx, conn, "chat_messages", 2)

	assertNoOlderThan(t, ctx, conn, "logs", "ts", toMs(now.Add(-24*time.Hour)))
	assertNoOlderThan(t, ctx, conn, "task_invocations", "created_at", toMs(now.Add(-24*time.Hour)))
	assertNoOlderThan(t, ctx, conn, "secretary_events", "ts", toMs(now.Add(-24*time.Hour)))
	assertNoOlderThan(t, ctx, conn, "secretary_compressions", "ts", toMs(now.Add(-24*time.Hour)))
	assertNoOlderThan(t, ctx, conn, "chat_messages", "ts", toMs(now.Add(-24*time.Hour)))
}

func TestService_RunGCContinuesOnSourceFailure(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	now := time.Date(2026, 2, 8, 19, 0, 0, 0, time.UTC)
	mustInsertTask(t, ctx, conn, "task-x", now)
	for i := range 4 {
		mustExecDB(t, ctx, conn, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-x", toMs(now.Add(time.Duration(i)*time.Minute)), "system", "x")
	}

	mustExecDB(t, ctx, conn, `DROP TABLE secretary_events;`)

	svc := NewService(conn, Options{
		Now: func() time.Time { return now },
		Retention: RetentionOptions{
			Days:       90,
			MaxRows:    2,
			GCInterval: time.Hour,
		},
	})

	status := svc.RunGC(ctx)
	if len(status.Results) == 0 {
		t.Fatalf("expected results")
	}

	foundErr := false
	for _, item := range status.Results {
		if item.Source == SourceSecretaryEvent && item.Error != "" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Fatalf("expected secretary_event source error in gc status: %+v", status.Results)
	}

	assertCountLE(t, ctx, conn, "logs", 2)
}

func assertCountLE(t *testing.T, ctx context.Context, conn queryRower, table string, max int) {
	t.Helper()
	row := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+`;`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if n > max {
		t.Fatalf("table %s count=%d want <=%d", table, n, max)
	}
}

func assertNoOlderThan(t *testing.T, ctx context.Context, conn queryRower, table string, col string, cutoffMs int64) {
	t.Helper()
	row := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` < ?;`, cutoffMs)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count old %s: %v", table, err)
	}
	if n != 0 {
		t.Fatalf("table %s has %d rows older than cutoff", table, n)
	}
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

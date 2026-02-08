package audit

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func toMs(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func mustExecDB(t *testing.T, ctx context.Context, conn *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery=%s", err, query)
	}
}

func mustInsertTask(t *testing.T, ctx context.Context, conn *sql.DB, id string, ts time.Time) {
	t.Helper()
	millis := toMs(ts.UTC())
	mustExecDB(
		t,
		ctx,
		conn,
		`INSERT INTO tasks(id, worker_type, mode, status, prompt, workdir, session_id, conversation_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		id, "exec", "new", "succeeded", "audit-seed", "/tmp", "", "conv-"+id, millis, millis,
	)
}

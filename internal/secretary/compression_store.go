package secretary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/chat"
)

type CompressionStore struct {
	db  *sql.DB
	now func() time.Time
}

type CompressionRecord struct {
	ID             int64
	Time           time.Time
	ConversationID string
	CursorBefore   int64
	CursorAfter    int64
	KeepFrom       int64
	Summary        string
	Backend        string
	Error          string
}

func NewCompressionStore(db *sql.DB) *CompressionStore {
	return &CompressionStore{db: db, now: time.Now}
}

func (s *CompressionStore) Append(ctx context.Context, rec CompressionRecord) (CompressionRecord, error) {
	return s.AppendInConversation(ctx, rec.ConversationID, rec)
}

func (s *CompressionStore) AppendInConversation(ctx context.Context, conversationID string, rec CompressionRecord) (CompressionRecord, error) {
	if s == nil || s.db == nil {
		return CompressionRecord{}, errors.New("secretary: compression store not initialized")
	}
	conversationID = chat.NormalizeConversationID(conversationID)
	ts := rec.Time
	if ts.IsZero() {
		ts = s.now()
	}
	ts = ts.UTC()

	summary := strings.TrimSpace(rec.Summary)
	backend := strings.TrimSpace(rec.Backend)
	errStr := strings.TrimSpace(rec.Error)

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO secretary_compressions (ts, cursor_before, cursor_after, keep_from, summary, backend, error, conversation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`, toMillis(ts), rec.CursorBefore, rec.CursorAfter, rec.KeepFrom, summary, backend, errStr, conversationID)
	if err != nil {
		return CompressionRecord{}, fmt.Errorf("secretary: compression append: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CompressionRecord{}, fmt.Errorf("secretary: compression id: %w", err)
	}
	out := rec
	out.ID = id
	out.Time = ts
	out.ConversationID = conversationID
	out.Summary = summary
	out.Backend = backend
	out.Error = errStr
	return out, nil
}

func (s *CompressionStore) Latest(ctx context.Context) (CompressionRecord, bool, error) {
	return s.LatestInConversation(ctx, "")
}

func (s *CompressionStore) LatestInConversation(ctx context.Context, conversationID string) (CompressionRecord, bool, error) {
	if s == nil || s.db == nil {
		return CompressionRecord{}, false, errors.New("secretary: compression store not initialized")
	}
	conversationID = chat.NormalizeConversationID(conversationID)
	var (
		rec      CompressionRecord
		tsMillis int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, ts, cursor_before, cursor_after, keep_from, summary, backend, error, conversation_id
		FROM secretary_compressions
		WHERE conversation_id = ?
		ORDER BY id DESC
		LIMIT 1;
	`, conversationID).Scan(&rec.ID, &tsMillis, &rec.CursorBefore, &rec.CursorAfter, &rec.KeepFrom, &rec.Summary, &rec.Backend, &rec.Error, &rec.ConversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CompressionRecord{}, false, nil
		}
		return CompressionRecord{}, false, fmt.Errorf("secretary: compression latest: %w", err)
	}
	rec.Time = fromMillis(tsMillis)
	return rec, true, nil
}

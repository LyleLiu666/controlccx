package audit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type pageCursor struct {
	TimeMs int64  `json:"t"`
	ID     string `json:"id"`
}

func encodeCursor(entry Entry) string {
	payload, err := json.Marshal(pageCursor{
		TimeMs: toMillis(entry.Time),
		ID:     strings.TrimSpace(entry.ID),
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(raw string) (pageCursor, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return pageCursor{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, fmt.Errorf("%w: decode base64: %v", ErrInvalidCursor, err)
	}
	var cur pageCursor
	if err := json.Unmarshal(data, &cur); err != nil {
		return pageCursor{}, fmt.Errorf("%w: decode payload: %v", ErrInvalidCursor, err)
	}
	if cur.TimeMs <= 0 || strings.TrimSpace(cur.ID) == "" {
		return pageCursor{}, fmt.Errorf("%w: missing fields", ErrInvalidCursor)
	}
	return cur, nil
}

func isOlderThanCursor(entry Entry, cursor pageCursor) bool {
	ct := fromMillis(cursor.TimeMs)
	if entry.Time.Before(ct) {
		return true
	}
	if entry.Time.After(ct) {
		return false
	}
	return entry.ID < cursor.ID
}

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func fromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}

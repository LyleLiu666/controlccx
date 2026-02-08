package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"controlccx/internal/audit"
)

func (a *API) handleAuditEntries(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if a.Audit == nil {
		http.Error(w, "audit not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sources, err := parseAuditSources(r.URL.Query().Get("sources"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	from, err := parseAuditTime(r.URL.Query().Get("from"))
	if err != nil {
		http.Error(w, "invalid from", http.StatusBadRequest)
		return
	}
	to, err := parseAuditTime(r.URL.Query().Get("to"))
	if err != nil {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}

	result, err := a.Audit.Query(r.Context(), audit.Query{
		Sources: sources,
		Q:       strings.TrimSpace(r.URL.Query().Get("q")),
		From:    from,
		To:      to,
		TaskID:  strings.TrimSpace(r.URL.Query().Get("task_id")),
		RunID:   strings.TrimSpace(r.URL.Query().Get("run_id")),
		Streams: parseStreams(r.URL.Query().Get("streams")),
		Limit:   parseInt(r.URL.Query().Get("limit"), 100),
		Cursor:  strings.TrimSpace(r.URL.Query().Get("cursor")),
	})
	if err != nil {
		if errors.Is(err, audit.ErrInvalidSource) || errors.Is(err, audit.ErrInvalidCursor) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func (a *API) handleAuditEntryByID(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if a.Audit == nil {
		http.Error(w, "audit not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entryID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/audit/entries/"))
	if entryID == "" {
		http.NotFound(w, r)
		return
	}
	detail, err := a.Audit.GetEntry(r.Context(), entryID)
	if err != nil {
		if errors.Is(err, audit.ErrEntryNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, detail)
}

func (a *API) handleAuditSources(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if a.Audit == nil {
		http.Error(w, "audit not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"sources": a.Audit.Sources()})
}

func (a *API) handleAuditRetention(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if a.Audit == nil {
		http.Error(w, "audit not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.Audit.Retention())
}

func parseAuditSources(raw string) ([]audit.Source, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	var out []audit.Source
	seen := map[audit.Source]bool{}
	for _, part := range strings.Split(value, ",") {
		source := audit.Source(strings.TrimSpace(part))
		if source == "" {
			continue
		}
		if seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	return out, nil
}

func parseAuditTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	if ms, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(0, ms*int64(time.Millisecond)).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultRetentionDays    = 90
	defaultRetentionMaxRows = 500000
	defaultRetentionTick    = time.Hour
)

type retentionTarget struct {
	source    Source
	table     string
	timeCol   string
	idCol     string
	integerID bool
}

var retentionTargets = []retentionTarget{
	{source: SourceTaskLog, table: "logs", timeCol: "ts", idCol: "id", integerID: true},
	{source: SourceTaskTrace, table: "task_invocations", timeCol: "created_at", idCol: "task_id", integerID: false},
	{source: SourceSecretaryEvent, table: "secretary_events", timeCol: "ts", idCol: "id", integerID: true},
	{source: SourceSecretaryCompression, table: "secretary_compressions", timeCol: "ts", idCol: "id", integerID: true},
	{source: SourceSecretaryChat, table: "chat_messages", timeCol: "ts", idCol: "id", integerID: true},
}

func normalizeRetentionOptions(in RetentionOptions) RetentionOptions {
	out := in
	if out.Days <= 0 {
		out.Days = defaultRetentionDays
	}
	if out.MaxRows <= 0 {
		out.MaxRows = defaultRetentionMaxRows
	}
	if out.GCInterval <= 0 {
		out.GCInterval = defaultRetentionTick
	}
	if !out.StartupRunGC {
		// Keep explicit false only when caller set a non-zero config intentionally.
		// In practice Options zero-value should still run startup GC.
		if in.Days == 0 && in.MaxRows == 0 && in.GCInterval == 0 && !in.StartupRunGC {
			out.StartupRunGC = true
		}
	}
	return out
}

func (s *Service) RunGC(ctx context.Context) GCStatus {
	if s == nil || s.db == nil {
		return GCStatus{}
	}
	startWall := time.Now()
	runAt := s.now().UTC()
	cutoffMs := toMillis(runAt.Add(-time.Duration(s.retention.Days) * 24 * time.Hour))

	status := GCStatus{
		RunAt:   runAt,
		Results: make([]GCSourceResult, 0, len(retentionTargets)),
	}

	for _, target := range retentionTargets {
		item := GCSourceResult{
			Source: target.source,
			Table:  target.table,
		}

		deletedByAge, err := s.pruneByAge(ctx, target, cutoffMs)
		if err != nil {
			item.Error = err.Error()
			status.Results = append(status.Results, item)
			continue
		}
		item.DeletedByAge = deletedByAge

		deletedByCount, err := s.pruneByCount(ctx, target, s.retention.MaxRows)
		if err != nil {
			item.Error = err.Error()
			status.Results = append(status.Results, item)
			continue
		}
		item.DeletedByCount = deletedByCount
		status.Results = append(status.Results, item)
	}

	status.DurationMs = int64(time.Since(startWall) / time.Millisecond)

	s.gcMu.Lock()
	copyStatus := status
	copyStatus.Results = append([]GCSourceResult(nil), status.Results...)
	s.gcLast = &copyStatus
	s.gcMu.Unlock()

	return status
}

func (s *Service) pruneByAge(ctx context.Context, target retentionTarget, cutoffMs int64) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s < ?;", target.table, target.timeCol)
	res, err := s.db.ExecContext(ctx, query, cutoffMs)
	if err != nil {
		return 0, fmt.Errorf("age prune %s: %w", target.table, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("age prune %s affected: %w", target.table, err)
	}
	return affected, nil
}

func (s *Service) pruneByCount(ctx context.Context, target retentionTarget, keep int) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}

	var cutoffTs int64
	if target.integerID {
		var cutoffID int64
		row := s.db.QueryRowContext(
			ctx,
			fmt.Sprintf(
				`SELECT %s, %s FROM %s ORDER BY %s DESC, %s DESC LIMIT 1 OFFSET ?;`,
				target.timeCol, target.idCol, target.table, target.timeCol, target.idCol,
			),
			keep-1,
		)
		if err := row.Scan(&cutoffTs, &cutoffID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, nil
			}
			return 0, fmt.Errorf("count prune cutoff %s: %w", target.table, err)
		}

		delQuery := fmt.Sprintf(
			`DELETE FROM %s WHERE %s < ? OR (%s = ? AND %s < ?);`,
			target.table, target.timeCol, target.timeCol, target.idCol,
		)
		res, err := s.db.ExecContext(ctx, delQuery, cutoffTs, cutoffTs, cutoffID)
		if err != nil {
			return 0, fmt.Errorf("count prune %s: %w", target.table, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count prune %s affected: %w", target.table, err)
		}
		return affected, nil
	}

	var cutoffID string
	row := s.db.QueryRowContext(
		ctx,
		fmt.Sprintf(
			`SELECT %s, %s FROM %s ORDER BY %s DESC, %s DESC LIMIT 1 OFFSET ?;`,
			target.timeCol, target.idCol, target.table, target.timeCol, target.idCol,
		),
		keep-1,
	)
	if err := row.Scan(&cutoffTs, &cutoffID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("count prune cutoff %s: %w", target.table, err)
	}

	delQuery := fmt.Sprintf(
		`DELETE FROM %s WHERE %s < ? OR (%s = ? AND %s < ?);`,
		target.table, target.timeCol, target.timeCol, target.idCol,
	)
	res, err := s.db.ExecContext(ctx, delQuery, cutoffTs, cutoffTs, cutoffID)
	if err != nil {
		return 0, fmt.Errorf("count prune %s: %w", target.table, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count prune %s affected: %w", target.table, err)
	}
	return affected, nil
}

func StartGCLoop(service *Service, logger func(format string, args ...any)) func() {
	if service == nil {
		return func() {}
	}
	logf := logger
	if logf == nil {
		logf = func(string, ...any) {}
	}

	logStatus := func(status GCStatus) {
		if status.RunAt.IsZero() {
			return
		}
		var (
			ageDeleted   int64
			countDeleted int64
			errorsN      int
		)
		for _, item := range status.Results {
			ageDeleted += item.DeletedByAge
			countDeleted += item.DeletedByCount
			if strings.TrimSpace(item.Error) != "" {
				errorsN++
			}
		}
		logf(
			"audit gc: run_at=%s duration_ms=%d deleted_by_age=%d deleted_by_count=%d errors=%d",
			status.RunAt.Format(time.RFC3339),
			status.DurationMs,
			ageDeleted,
			countDeleted,
			errorsN,
		)
		for _, item := range status.Results {
			if strings.TrimSpace(item.Error) == "" {
				continue
			}
			logf("audit gc: source=%s table=%s error=%s", item.Source, item.Table, item.Error)
		}
	}

	if service.retention.StartupRunGC {
		logStatus(service.RunGC(context.Background()))
	}
	if service.retention.GCInterval <= 0 {
		return func() {}
	}

	stopCh := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(service.retention.GCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logStatus(service.RunGC(context.Background()))
			case <-stopCh:
				return
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stopCh)
		})
	}
}

package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Options struct {
	Path string
}

func Open(ctx context.Context, opts Options) (*sql.DB, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("db: Path is required")
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", opts.Path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetConnMaxLifetime(0)
	conn.SetConnMaxIdleTime(0)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	if err := initPragmas(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := Migrate(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func initPragmas(ctx context.Context, conn *sql.DB) error {
	stmts := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, stmt := range stmts {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("db: pragma: %q: %w", stmt, err)
		}
	}
	return nil
}


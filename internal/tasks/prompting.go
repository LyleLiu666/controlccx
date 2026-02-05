package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ProjectContext struct {
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

const defaultProjectContextKey = "default"

func (s *Store) GetProjectContext(ctx context.Context) (ProjectContext, bool, error) {
	if s == nil || s.db == nil {
		return ProjectContext{}, false, errors.New("tasks: store not initialized")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT content, updated_at
		FROM project_context
		WHERE key = ?;
	`, defaultProjectContextKey)

	var (
		out       ProjectContext
		updatedAt int64
	)
	if err := row.Scan(&out.Content, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectContext{}, false, nil
		}
		return ProjectContext{}, false, fmt.Errorf("tasks: get project_context: %w", err)
	}
	out.Content = strings.TrimSpace(out.Content)
	out.UpdatedAt = fromMillis(updatedAt)
	return out, true, nil
}

func (s *Store) SetProjectContext(ctx context.Context, content string) (ProjectContext, error) {
	if s == nil || s.db == nil {
		return ProjectContext{}, errors.New("tasks: store not initialized")
	}
	content = strings.TrimSpace(content)
	now := toMillis(s.now().UTC())

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO project_context (key, content, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			content = excluded.content,
			updated_at = excluded.updated_at;
	`, defaultProjectContextKey, content, now); err != nil {
		return ProjectContext{}, fmt.Errorf("tasks: set project_context: %w", err)
	}

	out, _, err := s.GetProjectContext(ctx)
	if err != nil {
		return ProjectContext{}, err
	}
	return out, nil
}

type PromptTemplate struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertPromptTemplateInput struct {
	ID      string
	Title   string
	Kind    string
	Content string
}

func (s *Store) ListPromptTemplates(ctx context.Context, kind string) ([]PromptTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" || k == "all" {
		k = ""
	}
	if k != "" && k != "task" && k != "chat" {
		return nil, fmt.Errorf("tasks: invalid template kind %q", strings.TrimSpace(kind))
	}

	var (
		rows *sql.Rows
		err  error
	)
	if k == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, title, kind, content, created_at, updated_at
			FROM prompt_templates
			ORDER BY updated_at DESC, id DESC;
		`)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, title, kind, content, created_at, updated_at
			FROM prompt_templates
			WHERE kind = ?
			ORDER BY updated_at DESC, id DESC;
		`, k)
	}
	if err != nil {
		return nil, fmt.Errorf("tasks: list prompt_templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PromptTemplate
	for rows.Next() {
		var (
			t         PromptTemplate
			createdAt int64
			updatedAt int64
		)
		if err := rows.Scan(&t.ID, &t.Title, &t.Kind, &t.Content, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("tasks: scan prompt_templates: %w", err)
		}
		t.ID = strings.TrimSpace(t.ID)
		t.Title = strings.TrimSpace(t.Title)
		t.Kind = strings.TrimSpace(t.Kind)
		t.Content = strings.TrimSpace(t.Content)
		t.CreatedAt = fromMillis(createdAt)
		t.UpdatedAt = fromMillis(updatedAt)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list prompt_templates rows: %w", err)
	}
	return out, nil
}

func (s *Store) UpsertPromptTemplate(ctx context.Context, in UpsertPromptTemplateInput) (PromptTemplate, error) {
	if s == nil || s.db == nil {
		return PromptTemplate{}, errors.New("tasks: store not initialized")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return PromptTemplate{}, errors.New("tasks: template title is required")
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind != "task" && kind != "chat" {
		return PromptTemplate{}, fmt.Errorf("tasks: invalid template kind %q", strings.TrimSpace(in.Kind))
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return PromptTemplate{}, errors.New("tasks: template content is required")
	}

	now := toMillis(s.now().UTC())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO prompt_templates (id, title, kind, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			kind = excluded.kind,
			content = excluded.content,
			updated_at = excluded.updated_at;
	`, id, title, kind, content, now, now); err != nil {
		return PromptTemplate{}, fmt.Errorf("tasks: upsert prompt_templates: %w", err)
	}

	out, ok, err := s.getPromptTemplate(ctx, id)
	if err != nil {
		return PromptTemplate{}, err
	}
	if !ok {
		return PromptTemplate{}, fmt.Errorf("tasks: prompt_templates upsert succeeded but row not found (id=%s)", id)
	}
	return out, nil
}

func (s *Store) DeletePromptTemplate(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tasks: template id is required")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM prompt_templates WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("tasks: delete prompt_templates: %w", err)
	}
	return nil
}

func (s *Store) getPromptTemplate(ctx context.Context, id string) (PromptTemplate, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PromptTemplate{}, false, errors.New("tasks: template id is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, kind, content, created_at, updated_at
		FROM prompt_templates
		WHERE id = ?;
	`, id)

	var (
		out       PromptTemplate
		createdAt int64
		updatedAt int64
	)
	if err := row.Scan(&out.ID, &out.Title, &out.Kind, &out.Content, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromptTemplate{}, false, nil
		}
		return PromptTemplate{}, false, fmt.Errorf("tasks: get prompt_templates: %w", err)
	}
	out.ID = strings.TrimSpace(out.ID)
	out.Title = strings.TrimSpace(out.Title)
	out.Kind = strings.TrimSpace(out.Kind)
	out.Content = strings.TrimSpace(out.Content)
	out.CreatedAt = fromMillis(createdAt)
	out.UpdatedAt = fromMillis(updatedAt)
	return out, true, nil
}

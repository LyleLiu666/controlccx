package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type MissionContract struct {
	Key                string    `json:"key"`
	Goal               string    `json:"goal"`
	Constraints        []string  `json:"constraints"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	NonGoals           []string  `json:"non_goals"`
	Revision           int       `json:"revision"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type UpsertMissionContractInput struct {
	Key                string
	Goal               string
	Constraints        []string
	AcceptanceCriteria []string
	NonGoals           []string
}

func (s *Store) GetMissionContract(ctx context.Context, key string) (MissionContract, bool, error) {
	if s == nil || s.db == nil {
		return MissionContract{}, false, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return MissionContract{}, false, errors.New("tasks: mission contract key is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT key, goal, constraints_json, acceptance_json, non_goals_json, revision, created_at, updated_at
		FROM mission_contracts
		WHERE key = ?;
	`, key)

	var (
		out                      MissionContract
		constraintsJSON          string
		acceptanceJSON           string
		nonGoalsJSON             string
		createdAtMs, updatedAtMs int64
	)
	if err := row.Scan(
		&out.Key,
		&out.Goal,
		&constraintsJSON,
		&acceptanceJSON,
		&nonGoalsJSON,
		&out.Revision,
		&createdAtMs,
		&updatedAtMs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MissionContract{}, false, nil
		}
		return MissionContract{}, false, fmt.Errorf("tasks: get mission contract: %w", err)
	}

	var err error
	if out.Constraints, err = decodeMissionStringList(constraintsJSON); err != nil {
		return MissionContract{}, false, fmt.Errorf("tasks: decode mission contract constraints: %w", err)
	}
	if out.AcceptanceCriteria, err = decodeMissionStringList(acceptanceJSON); err != nil {
		return MissionContract{}, false, fmt.Errorf("tasks: decode mission contract acceptance criteria: %w", err)
	}
	if out.NonGoals, err = decodeMissionStringList(nonGoalsJSON); err != nil {
		return MissionContract{}, false, fmt.Errorf("tasks: decode mission contract non-goals: %w", err)
	}
	out.Goal = strings.TrimSpace(out.Goal)
	out.CreatedAt = fromMillis(createdAtMs)
	out.UpdatedAt = fromMillis(updatedAtMs)
	return out, true, nil
}

func (s *Store) UpsertMissionContract(ctx context.Context, in UpsertMissionContractInput) (MissionContract, error) {
	if s == nil || s.db == nil {
		return MissionContract{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return MissionContract{}, errors.New("tasks: mission contract key is required")
	}
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return MissionContract{}, errors.New("tasks: mission contract goal is required")
	}

	constraintsJSON, err := encodeMissionStringList(in.Constraints)
	if err != nil {
		return MissionContract{}, fmt.Errorf("tasks: encode mission contract constraints: %w", err)
	}
	acceptanceJSON, err := encodeMissionStringList(in.AcceptanceCriteria)
	if err != nil {
		return MissionContract{}, fmt.Errorf("tasks: encode mission contract acceptance criteria: %w", err)
	}
	nonGoalsJSON, err := encodeMissionStringList(in.NonGoals)
	if err != nil {
		return MissionContract{}, fmt.Errorf("tasks: encode mission contract non-goals: %w", err)
	}

	nowMs := toMillis(s.now().UTC())
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO mission_contracts (
			key, goal, constraints_json, acceptance_json, non_goals_json, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			goal = excluded.goal,
			constraints_json = excluded.constraints_json,
			acceptance_json = excluded.acceptance_json,
			non_goals_json = excluded.non_goals_json,
			revision = mission_contracts.revision + 1,
			updated_at = excluded.updated_at;
	`, key, goal, constraintsJSON, acceptanceJSON, nonGoalsJSON, nowMs, nowMs)
	if err != nil {
		return MissionContract{}, fmt.Errorf("tasks: upsert mission contract: %w", err)
	}
	out, _, err := s.GetMissionContract(ctx, key)
	if err != nil {
		return MissionContract{}, err
	}
	return out, nil
}

func encodeMissionStringList(values []string) (string, error) {
	norm := normalizeMissionStringList(values)
	if len(norm) == 0 {
		norm = []string{}
	}
	b, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeMissionStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return normalizeMissionStringList(values), nil
}

func normalizeMissionStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		item := strings.TrimSpace(v)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

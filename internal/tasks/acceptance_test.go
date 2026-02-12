package tasks

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_AcceptanceState_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	fixedNow := time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixedNow }

	_, ok, err := store.GetAcceptanceState(ctx, "s:sess-123")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if ok {
		t.Fatalf("expected missing acceptance state")
	}

	state, err := store.UpsertAcceptanceState(ctx, UpsertAcceptanceStateInput{
		Key:           "s:sess-123",
		Status:        "running",
		Iteration:     3,
		MaxIterations: 10,
		CurrentGate:   "runnability.smoke",
		Summary:       "waiting for resume run",
		PlanJSON:      `{"intent":"runnable"}`,
		Report:        "",
		RunID:         "run-abc",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if state.Key != "s:sess-123" {
		t.Fatalf("key=%q", state.Key)
	}
	if state.Status != "running" {
		t.Fatalf("status=%q", state.Status)
	}
	if state.Iteration != 3 {
		t.Fatalf("iteration=%d", state.Iteration)
	}
	if state.MaxIterations != 10 {
		t.Fatalf("max_iterations=%d", state.MaxIterations)
	}
	if state.UpdatedAt != fixedNow {
		t.Fatalf("updated_at=%s, want %s", state.UpdatedAt, fixedNow)
	}

	state2, ok, err := store.GetAcceptanceState(ctx, "s:sess-123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected acceptance state present")
	}
	if state2.Iteration != 3 || state2.CurrentGate != "runnability.smoke" {
		t.Fatalf("unexpected state=%+v", state2)
	}

	store.now = func() time.Time { return fixedNow.Add(2 * time.Minute) }
	state3, err := store.UpsertAcceptanceState(ctx, UpsertAcceptanceStateInput{
		Key:         "s:sess-123",
		Status:      "accepted",
		Iteration:   4,
		CurrentGate: "done",
		Summary:     "all gates passed",
		Report:      "OK",
		RunID:       "run-def",
	})
	if err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	if state3.MaxIterations != 10 {
		t.Fatalf("expected default max_iterations=10, got %d", state3.MaxIterations)
	}
	if state3.UpdatedAt != fixedNow.Add(2*time.Minute) {
		t.Fatalf("updated_at=%s", state3.UpdatedAt)
	}
}

func TestStore_AcceptanceState_BridgesMissionContractCriteriaIntoPlan(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	key := "c:conv-bridge"

	if _, err := store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  key,
		Goal: "ship safely",
		AcceptanceCriteria: []string{
			"go test ./... passes",
			"文档可读性高，适合团队接手",
		},
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}

	state, err := store.UpsertAcceptanceState(ctx, UpsertAcceptanceStateInput{
		Key:       key,
		Status:    "running",
		Iteration: 1,
		Summary:   "bridged",
	})
	if err != nil {
		t.Fatalf("upsert acceptance state: %v", err)
	}
	if state.PlanJSON == "" {
		t.Fatalf("plan_json empty, want bridged criteria plan")
	}

	var plan struct {
		ContractRevision int `json:"contract_revision"`
		Criteria         []struct {
			CriterionID string `json:"criterion_id"`
			GateType    string `json:"gate_type"`
		} `json:"criteria"`
	}
	if err := json.Unmarshal([]byte(state.PlanJSON), &plan); err != nil {
		t.Fatalf("decode plan_json: %v", err)
	}
	if plan.ContractRevision != 1 {
		t.Fatalf("contract_revision=%d, want 1", plan.ContractRevision)
	}
	if len(plan.Criteria) != 2 {
		t.Fatalf("criteria len=%d, want 2", len(plan.Criteria))
	}
	if plan.Criteria[0].CriterionID != "ac-001" || plan.Criteria[1].CriterionID != "ac-002" {
		t.Fatalf("criterion ids=%+v, want ac-001/ac-002", plan.Criteria)
	}
	if plan.Criteria[0].GateType != "objective" {
		t.Fatalf("criteria[0].gate_type=%q, want objective", plan.Criteria[0].GateType)
	}
	if plan.Criteria[1].GateType != "subjective" {
		t.Fatalf("criteria[1].gate_type=%q, want subjective", plan.Criteria[1].GateType)
	}
}

func TestStore_AcceptanceState_BridgeDoesNotOverrideExplicitPlan(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	key := "c:conv-bridge-explicit"
	if _, err := store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:                key,
		Goal:               "ship safely",
		AcceptanceCriteria: []string{"go test ./... passes"},
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}

	explicit := `{"manual":true}`
	state, err := store.UpsertAcceptanceState(ctx, UpsertAcceptanceStateInput{
		Key:       key,
		Status:    "running",
		Iteration: 1,
		PlanJSON:  explicit,
	})
	if err != nil {
		t.Fatalf("upsert acceptance state: %v", err)
	}
	if state.PlanJSON != explicit {
		t.Fatalf("plan_json=%q, want explicit %q", state.PlanJSON, explicit)
	}
}

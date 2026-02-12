package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	acceptanceObjectiveNumberRE = regexp.MustCompile(`\d`)
)

type acceptanceContractPlan struct {
	Source           string                           `json:"source"`
	ContractKey      string                           `json:"contract_key"`
	ContractRevision int                              `json:"contract_revision"`
	Criteria         []acceptanceContractPlanCriteria `json:"criteria"`
}

type acceptanceContractPlanCriteria struct {
	CriterionID string `json:"criterion_id"`
	Criterion   string `json:"criterion"`
	GateType    string `json:"gate_type"`
}

func (s *Store) bridgeAcceptancePlanFromMissionContract(ctx context.Context, key string) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, nil
	}
	contract, ok, err := s.GetMissionContract(ctx, key)
	if err != nil {
		return "", false, fmt.Errorf("tasks: bridge acceptance from mission contract: %w", err)
	}
	if !ok || len(contract.AcceptanceCriteria) == 0 {
		return "", false, nil
	}

	criteria := make([]acceptanceContractPlanCriteria, 0, len(contract.AcceptanceCriteria))
	for i, raw := range contract.AcceptanceCriteria {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		criteria = append(criteria, acceptanceContractPlanCriteria{
			CriterionID: fmt.Sprintf("ac-%03d", i+1),
			Criterion:   text,
			GateType:    classifyGateTypeFromCriterion(text),
		})
	}
	if len(criteria) == 0 {
		return "", false, nil
	}

	plan := acceptanceContractPlan{
		Source:           "mission_contract_acceptance_bridge_v1",
		ContractKey:      strings.TrimSpace(contract.Key),
		ContractRevision: contract.Revision,
		Criteria:         criteria,
	}
	b, err := json.Marshal(plan)
	if err != nil {
		return "", false, fmt.Errorf("tasks: bridge acceptance from mission contract marshal: %w", err)
	}
	return string(b), true, nil
}

func classifyGateTypeFromCriterion(criterion string) string {
	lower := strings.ToLower(strings.TrimSpace(criterion))
	if lower == "" {
		return "subjective"
	}

	// Strong objective signals: tests/build/lint/smoke, explicit numeric targets or comparisons.
	if strings.Contains(lower, "go test") ||
		strings.Contains(lower, "pnpm test") ||
		strings.Contains(lower, "npm test") ||
		strings.Contains(lower, "build") ||
		strings.Contains(lower, "lint") ||
		strings.Contains(lower, "smoke") ||
		strings.Contains(lower, "passes") ||
		strings.Contains(lower, "must pass") ||
		strings.Contains(lower, ">=") ||
		strings.Contains(lower, "<=") ||
		strings.Contains(lower, "至少") ||
		strings.Contains(lower, "不少于") ||
		acceptanceObjectiveNumberRE.MatchString(lower) {
		return "objective"
	}

	// Common subjective wording.
	if strings.Contains(lower, "readab") ||
		strings.Contains(lower, "quality") ||
		strings.Contains(lower, "clear") ||
		strings.Contains(lower, "maintainab") ||
		strings.Contains(lower, "可读") ||
		strings.Contains(lower, "易读") ||
		strings.Contains(lower, "质量") ||
		strings.Contains(lower, "体验") ||
		strings.Contains(lower, "风格") {
		return "subjective"
	}

	// Conservative default: subjective unless clearly measurable.
	return "subjective"
}

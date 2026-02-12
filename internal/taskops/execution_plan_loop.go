package taskops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"controlccx/internal/tasks"
)

type RunExecutionPlanLoopInput struct {
	MaxIterations   int    `json:"max_iterations,omitempty"`
	IterationPrompt string `json:"iteration_prompt,omitempty"`
}

type RunExecutionPlanLoopResult struct {
	Key                 string                   `json:"key"`
	ConversationID      string                   `json:"conversation_id"`
	IterationsRequested int                      `json:"iterations_requested"`
	IterationsExecuted  int                      `json:"iterations_executed"`
	NextAction          tasks.NextAction         `json:"next_action"`
	State               tasks.ExecutionPlanState `json:"state"`
	LastTaskID          string                   `json:"last_task_id,omitempty"`
}

type executionPlanV1 struct {
	Version string                `json:"version"`
	Goal    string                `json:"goal"`
	Steps   []executionPlanStepV1 `json:"steps"`
}

type executionPlanStepV1 struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s *Service) RunExecutionPlanLoopV1(ctx context.Context, key string, in RunExecutionPlanLoopInput) (RunExecutionPlanLoopResult, error) {
	if s == nil || s.Tasks == nil {
		return RunExecutionPlanLoopResult{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return RunExecutionPlanLoopResult{}, newMutationError(400, MutationErrorInvalidArgument, "session key is required", "", nil, nil)
	}

	maxIterations := in.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 1
	}
	if maxIterations > 10 {
		maxIterations = 10
	}

	conversationID, err := resolveConversationIDForSessionKey(ctx, s.Tasks, key)
	if err != nil {
		return RunExecutionPlanLoopResult{}, err
	}
	contractKey := tasks.ConversationKey(conversationID)
	contract, ok, err := s.Tasks.GetMissionContract(ctx, contractKey)
	if err != nil {
		return RunExecutionPlanLoopResult{}, err
	}
	if !ok {
		return RunExecutionPlanLoopResult{}, newMutationError(409, MutationErrorUnsupported, "mission contract is required before autonomous loop", "", map[string]any{
			"session_key": key,
		}, nil)
	}
	confirmed, err := s.Tasks.IsMissionContractRevisionConfirmed(ctx, contractKey, contract.Revision)
	if err != nil {
		return RunExecutionPlanLoopResult{}, err
	}
	if !confirmed {
		return RunExecutionPlanLoopResult{}, newMutationError(409, MutationErrorUnsupported, "cannot continue until mission contract is confirmed", "", map[string]any{
			"contract_key": contractKey,
			"revision":     contract.Revision,
		}, nil)
	}

	state, err := s.ensureExecutionPlanState(ctx, contract)
	if err != nil {
		return RunExecutionPlanLoopResult{}, err
	}

	result := RunExecutionPlanLoopResult{
		Key:                 key,
		ConversationID:      conversationID,
		IterationsRequested: maxIterations,
		State:               state,
	}

	for i := 0; i < maxIterations; i++ {
		next, err := s.Tasks.ComputeNextAction(ctx, conversationID)
		if err != nil {
			return result, err
		}
		result.NextAction = next

		switch next.Action {
		case tasks.NextActionStartRun, tasks.NextActionResumeRun:
			prompt := strings.TrimSpace(in.IterationPrompt)
			if prompt == "" {
				prompt = defaultExecutionPlanIterationPrompt(state.PlanJSON, state.Iteration, contract.Goal)
			}
			continueOut, err := s.ContinueSession(ctx, key, RunOptions{Prompt: prompt})
			if err != nil {
				return result, err
			}

			state.LastAction = strings.TrimSpace(string(next.Action))
			state.Status = "running"
			progressed := false
			if continueOut.Task != nil {
				state.Iteration++
				progressed = true
				state.LastTaskID = strings.TrimSpace(continueOut.Task.ID)
			}
			if continueOut.Queue != nil {
				state.Status = "queued"
				if tid := strings.TrimSpace(continueOut.Queue.ExistingTaskID); tid != "" {
					state.LastTaskID = tid
				}
			}
			// Keep progress monotonic only when a new run was actually created.
			if continueOut.Task == nil && continueOut.Queue == nil {
				state.Iteration++
				progressed = true
			}

			state, err = s.persistExecutionPlanState(ctx, state)
			if err != nil {
				return result, err
			}
			result.State = state
			result.LastTaskID = strings.TrimSpace(state.LastTaskID)
			if progressed {
				result.IterationsExecuted++
			}

			// If the action was queued, stop this loop tick and wait for scheduler progress.
			if continueOut.Queue != nil {
				return result, nil
			}
		default:
			state.LastAction = strings.TrimSpace(string(next.Action))
			state.Status = "waiting"
			state, err = s.persistExecutionPlanState(ctx, state)
			if err != nil {
				return result, err
			}
			result.State = state
			return result, nil
		}
	}

	if next, err := s.Tasks.ComputeNextAction(ctx, conversationID); err == nil {
		result.NextAction = next
	}
	return result, nil
}

func (s *Service) ensureExecutionPlanState(ctx context.Context, contract tasks.MissionContract) (tasks.ExecutionPlanState, error) {
	key := strings.TrimSpace(contract.Key)
	if key == "" {
		return tasks.ExecutionPlanState{}, fmt.Errorf("execution plan key is required")
	}
	state, ok, err := s.Tasks.GetExecutionPlanState(ctx, key)
	if err != nil {
		return tasks.ExecutionPlanState{}, err
	}
	if ok && state.MissionRevision == contract.Revision && strings.TrimSpace(state.PlanJSON) != "" {
		return state, nil
	}

	planJSON, err := buildExecutionPlanJSONV1(contract)
	if err != nil {
		return tasks.ExecutionPlanState{}, err
	}
	return s.Tasks.UpsertExecutionPlanState(ctx, tasks.UpsertExecutionPlanStateInput{
		Key:             key,
		MissionRevision: contract.Revision,
		PlanJSON:        planJSON,
		Iteration:       0,
		LastAction:      "",
		LastTaskID:      "",
		Status:          "planned",
	})
}

func (s *Service) persistExecutionPlanState(ctx context.Context, state tasks.ExecutionPlanState) (tasks.ExecutionPlanState, error) {
	return s.Tasks.UpsertExecutionPlanState(ctx, tasks.UpsertExecutionPlanStateInput{
		Key:             state.Key,
		MissionRevision: state.MissionRevision,
		PlanJSON:        state.PlanJSON,
		Iteration:       state.Iteration,
		LastAction:      state.LastAction,
		LastTaskID:      state.LastTaskID,
		Status:          state.Status,
	})
}

func buildExecutionPlanJSONV1(contract tasks.MissionContract) (string, error) {
	steps := selectExecutionPlanStepTitles(contract)
	plan := executionPlanV1{
		Version: "v1",
		Goal:    strings.TrimSpace(contract.Goal),
		Steps:   make([]executionPlanStepV1, 0, len(steps)),
	}
	for i, title := range steps {
		plan.Steps = append(plan.Steps, executionPlanStepV1{
			ID:    fmt.Sprintf("step-%03d", i+1),
			Title: strings.TrimSpace(title),
		})
	}
	b, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("marshal execution plan: %w", err)
	}
	return string(b), nil
}

func selectExecutionPlanStepTitles(contract tasks.MissionContract) []string {
	candidates := contract.AcceptanceCriteria
	if len(candidates) == 0 {
		candidates = contract.Constraints
	}
	if len(candidates) == 0 {
		candidates = []string{strings.TrimSpace(contract.Goal)}
	}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		out = append(out, "continue")
	}
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func defaultExecutionPlanIterationPrompt(planJSON string, iteration int, fallbackGoal string) string {
	title := executionPlanStepTitle(planJSON, iteration)
	if title == "" {
		title = strings.TrimSpace(fallbackGoal)
	}
	if title == "" {
		title = "continue"
	}
	return "按任务契约执行当前步骤：" + title
}

func executionPlanStepTitle(planJSON string, iteration int) string {
	if strings.TrimSpace(planJSON) == "" {
		return ""
	}
	var plan executionPlanV1
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return ""
	}
	if len(plan.Steps) == 0 {
		return ""
	}
	idx := iteration
	if idx < 0 {
		idx = 0
	}
	if idx >= len(plan.Steps) {
		idx = len(plan.Steps) - 1
	}
	return strings.TrimSpace(plan.Steps[idx].Title)
}

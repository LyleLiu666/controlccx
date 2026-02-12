package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

type nextActionExecuteBody struct {
	Action tasks.NextActionType `json:"action,omitempty"`
	Prompt string               `json:"prompt,omitempty"`
}

func decodeNextActionExecuteBody(body io.Reader) (nextActionExecuteBody, error) {
	var out nextActionExecuteBody
	if err := json.NewDecoder(body).Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return nextActionExecuteBody{}, err
	}
	return out, nil
}

func (a *API) handleSessionNextActionExecute(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := decodeNextActionExecuteBody(r.Body)
	if err != nil {
		writeTaskMutationInvalidJSON(w)
		return
	}

	conversationID, err := resolveConversationIDForSessionKey(r.Context(), a.Tasks, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	next, err := a.Tasks.ComputeNextAction(r.Context(), conversationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	execAction := next.Action
	if strings.TrimSpace(string(body.Action)) != "" {
		execAction = tasks.NextActionType(strings.TrimSpace(string(body.Action)))
	}

	switch execAction {
	case tasks.NextActionResumeRun, tasks.NextActionStartRun:
		ops := a.taskOpsOrShim()
		if ops == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		out, err := ops.ContinueSession(r.Context(), key, taskops.RunOptions{
			Prompt: strings.TrimSpace(body.Prompt),
		})
		if err != nil {
			writeTaskMutationProblem(w, err)
			return
		}
		meta := map[string]any{
			"next_action":        string(execAction),
			"recommended":        string(next.Action),
			"recommended_reason": next.Reason,
		}
		if out.Queue != nil {
			result := taskops.NewQueueMutationResult(taskops.ActionSessionNextActionExec, *out.Queue)
			result.Meta = meta
			writeTaskMutationResult(w, http.StatusAccepted, result)
			return
		}
		if out.Task != nil {
			result := taskops.NewTaskMutationResult(taskops.ActionSessionNextActionExec, *out.Task)
			result.Meta = meta
			writeTaskMutationResult(w, http.StatusOK, result)
			return
		}
		writeTaskMutationResult(w, http.StatusOK, taskops.MutationResult{
			OK:     true,
			Action: taskops.ActionSessionNextActionExec,
			Meta:   meta,
		})
		return
	default:
		writeTaskMutationProblem(w, errors.New("next action does not support execute api"))
		return
	}
}

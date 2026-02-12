package api

import "net/http"

func (a *API) handleSessionNextAction(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
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
	writeJSON(w, next)
}

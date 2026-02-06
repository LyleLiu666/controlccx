package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"controlccx/internal/daemon"
)

func proxySingleEndpoint(baseURL string, instanceToken string) http.HandlerFunc {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	instanceToken = strings.TrimSpace(instanceToken)
	return func(w http.ResponseWriter, r *http.Request) {
		if baseURL == "" {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "secretary_unavailable",
				"message": "upstream not configured",
				"hint":    "restart the secretary daemon (controlccx-secretaryd)",
			})
			return
		}
		if instanceToken == "" {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "secretary_unavailable",
				"message": "instance token not configured",
				"hint":    "restart ControlCCX",
			})
			return
		}

		target := baseURL + r.URL.RequestURI()
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Header = r.Header.Clone()
		req.Header.Set(daemon.InstanceTokenHeader, instanceToken)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "secretary_unavailable",
				"message": err.Error(),
				"hint":    "restart the secretary daemon (controlccx-secretaryd)",
			})
			return
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "secretary_unavailable",
				"message": "secretaryd authorization failed",
				"hint":    "ensure all ControlCCX processes use the same data dir (instance token mismatch)",
			})
			return
		}

		for k, vs := range res.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)

		if f, ok := w.(http.Flusher); ok {
			_, _ = io.Copy(flushWriter{w: w, f: f}, res.Body)
			return
		}
		_, _ = io.Copy(w, res.Body)
	}
}

type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

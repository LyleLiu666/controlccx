package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"controlccx/internal/daemon"
)

type controlPlaneComponent struct {
	OK              bool   `json:"ok"`
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
	PID             int    `json:"pid,omitempty"`
	Addr            string `json:"addr,omitempty"`
	Error           string `json:"error,omitempty"`
	TSMS            int64  `json:"ts_ms,omitempty"`
}

type controlPlaneStatus struct {
	Server     controlPlaneComponent `json:"server"`
	Runnerd    controlPlaneComponent `json:"runnerd"`
	Secretaryd controlPlaneComponent `json:"secretaryd"`
	TSMS       int64                 `json:"ts_ms"`
}

type daemonHealthPayload struct {
	OK              bool   `json:"ok"`
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
	PID             int    `json:"pid"`
	Addr            string `json:"addr"`
	TSMS            int64  `json:"ts_ms"`
}

func controlPlaneHandler(runnerBaseURL, secretaryBaseURL, instanceToken string) http.HandlerFunc {
	runnerBaseURL = strings.TrimRight(strings.TrimSpace(runnerBaseURL), "/")
	secretaryBaseURL = strings.TrimRight(strings.TrimSpace(secretaryBaseURL), "/")
	instanceToken = strings.TrimSpace(instanceToken)

	client := &http.Client{Timeout: 450 * time.Millisecond}

	fetch := func(ctx context.Context, baseURL string, fallbackName string) controlPlaneComponent {
		out := controlPlaneComponent{Name: strings.TrimSpace(fallbackName)}
		if baseURL == "" {
			out.Error = "not configured"
			return out
		}
		if instanceToken == "" {
			out.Error = "instance token not configured"
			return out
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			out.Error = err.Error()
			return out
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set(daemon.InstanceTokenHeader, instanceToken)

		res, err := client.Do(req)
		if err != nil {
			out.Error = err.Error()
			return out
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
			msg := strings.TrimSpace(string(body))
			if msg == "" {
				msg = http.StatusText(res.StatusCode)
			}
			out.Error = msg
			return out
		}

		var payload daemonHealthPayload
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload); err != nil {
			out.Error = err.Error()
			return out
		}

		out.OK = payload.OK
		if strings.TrimSpace(payload.Name) != "" {
			out.Name = strings.TrimSpace(payload.Name)
		}
		out.ProtocolVersion = payload.ProtocolVersion
		out.PID = payload.PID
		out.Addr = strings.TrimSpace(payload.Addr)
		out.TSMS = payload.TSMS

		if out.OK && out.ProtocolVersion != 0 && out.ProtocolVersion != daemon.ProtocolVersion {
			out.OK = false
			out.Error = "protocol mismatch"
		}
		return out
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		now := time.Now().UTC().UnixMilli()
		ctx, cancel := context.WithTimeout(r.Context(), 700*time.Millisecond)
		defer cancel()

		status := controlPlaneStatus{
			Server: controlPlaneComponent{
				OK:    true,
				Name:  "server",
				Addr:  strings.TrimSpace(r.Host),
				TSMS:  now,
				PID:   0,
				Error: "",
			},
			Runnerd:    fetch(ctx, runnerBaseURL, "runnerd"),
			Secretaryd: fetch(ctx, secretaryBaseURL, "secretaryd"),
			TSMS:       now,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

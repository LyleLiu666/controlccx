package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"controlccx/internal/daemon"
)

const tokenGateErrorCode = "instance_token_required"

func tokenRequiredForListenAddr(listenAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		// Safer default: if we cannot prove it's loopback-only, require a token.
		return true
	}

	host = strings.TrimSpace(host)
	if host == "" {
		// ":5174" binds all interfaces.
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback()
	}
	// Hostnames (non-IP) are routable by default.
	return true
}

func withInstanceTokenGate(listenAddr string, instanceToken string, next http.Handler) http.Handler {
	requireToken := tokenRequiredForListenAddr(listenAddr)
	instanceToken = strings.TrimSpace(instanceToken)

	if !requireToken {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if instanceToken == "" || !daemon.HasValidInstanceToken(r.Header, instanceToken) {
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
				"error":   tokenGateErrorCode,
				"message": "missing or invalid instance token (X-ControlCCX-Token)",
				"hint":    "find token at ~/.controlccx/instance.token and set it in the Web UI",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

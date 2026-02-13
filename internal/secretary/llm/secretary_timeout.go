package llm

import (
	"os"
	"strconv"
	"strings"
	"time"

	"controlccx/internal/config"
)

const (
	defaultSecretaryLLMTimeout = 30 * time.Minute
	secretaryLLMTimeoutEnvVar  = "CONTROLCCX_SECRETARY_LLM_TIMEOUT"
)

func resolveSecretaryLLMTimeout(cfg config.Config) time.Duration {
	if v := strings.TrimSpace(os.Getenv(secretaryLLMTimeoutEnvVar)); v != "" {
		if d, ok := parseSecretaryTimeout(v); ok {
			return d
		}
	}
	if v := strings.TrimSpace(cfg.Secretary.LLMTimeout); v != "" {
		if d, ok := parseSecretaryTimeout(v); ok {
			return d
		}
	}
	return defaultSecretaryLLMTimeout
}

func parseSecretaryTimeout(raw string) (time.Duration, bool) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return 0, false
	}
	switch v {
	case "off", "disable", "disabled", "none", "no", "false":
		return 0, true
	}

	// Treat all-digits values as seconds to avoid surprising "30" = 30ns.
	if isAllDigits(v) {
		sec, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		if sec <= 0 {
			return 0, true
		}
		return time.Duration(sec) * time.Second, true
	}

	dur, err := time.ParseDuration(v)
	if err != nil {
		return 0, false
	}
	if dur <= 0 {
		return 0, true
	}
	return dur, true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

package tasks

import (
	"os"
	"path/filepath"
	"strings"
)

func PopulateHints(t *Task) {
	if t == nil {
		return
	}
	t.FinishReason = FinishReason(*t)
	if isTerminalStatus(t.Status) {
		t.SuggestedTests = SuggestedTestCommands(t.WorkDir)
	}
}

func FinishReason(t Task) string {
	if !isTerminalStatus(t.Status) {
		return ""
	}

	if strings.TrimSpace(t.Error) != "" {
		return strings.TrimSpace(t.Error)
	}

	switch t.Status {
	case StatusSucceeded:
		return "succeeded"
	case StatusBlocked:
		return "blocked: approval required"
	case StatusCanceled:
		return "canceled"
	case StatusInterrupted:
		return "interrupted"
	case StatusFailed:
		if t.ExitCode != nil {
			return "failed: exit code " + itoa(*t.ExitCode)
		}
		return "failed"
	default:
		return string(t.Status)
	}
}

func SuggestedTestCommands(workdir string) []string {
	wd := strings.TrimSpace(workdir)
	if wd == "" {
		return nil
	}

	var out []string
	seen := map[string]bool{}
	add := func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || seen[cmd] {
			return
		}
		seen[cmd] = true
		out = append(out, cmd)
	}

	if fileExists(filepath.Join(wd, "go.mod")) {
		add("go test ./...")
	}

	// pnpm mono-repo style (e.g. this repo)
	if fileExists(filepath.Join(wd, "package.json")) && fileExists(filepath.Join(wd, "pnpm-lock.yaml")) {
		add("pnpm build")
		add("pnpm smoke")
	}
	if fileExists(filepath.Join(wd, "web", "package.json")) {
		add("pnpm -C web build")
	}

	return out
}

func isTerminalStatus(s Status) bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCanceled, StatusInterrupted, StatusBlocked:
		return true
	default:
		return false
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

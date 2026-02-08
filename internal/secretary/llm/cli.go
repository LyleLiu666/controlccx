package llm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/execenv"

	"github.com/goccy/go-json"
)

type MultiBackend struct {
	Backends []Backend
}

func (m MultiBackend) Name() string { return "multi" }

func (m MultiBackend) Complete(ctx context.Context, prompt string) (string, error) {
	var errs []string
	for _, b := range m.Backends {
		if b == nil {
			continue
		}
		out, err := b.Complete(ctx, prompt)
		if err == nil && strings.TrimSpace(out) != "" {
			return out, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", b.Name(), err))
		}
	}
	if len(errs) == 0 {
		return "", errors.New("no available LLM backends")
	}
	return "", fmt.Errorf("all LLM backends failed: %s", strings.Join(errs, "; "))
}

func NewDefaultCLIBackends(cfg config.Config, authStore *auth.Store) Backend {
	return MultiBackend{
		Backends: []Backend{
			NewClaudeCLIBackend(cfg, authStore),
			NewCodexCLIBackend(cfg, authStore),
		},
	}
}

type ClaudeCLIBackend struct {
	cfg  config.Config
	auth *auth.Store
}

func NewClaudeCLIBackend(cfg config.Config, authStore *auth.Store) Backend {
	return &ClaudeCLIBackend{cfg: cfg, auth: authStore}
}

func (b *ClaudeCLIBackend) Name() string { return "claude-cli" }

const slimClaudeSystemPrompt = ""

func (b *ClaudeCLIBackend) buildArgs() []string {
	return []string{
		"-p",
		"--tools", "",
		"--system-prompt", slimClaudeSystemPrompt,
		"--output-format", "stream-json",
		"--verbose",
		"-",
	}
}

func (b *ClaudeCLIBackend) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := withDefaultTimeout(ctx, 60*time.Second)
	defer cancel()

	cmdPath := strings.TrimSpace(b.cfg.Paths.Claude)
	if cmdPath == "" {
		cmdPath = "claude"
	}

	args := b.buildArgs()

	toolCmd := exec.CommandContext(ctx, cmdPath, args...)
	toolCmd.Dir = b.cfg.Paths.DataDir
	toolCmd.Env = mergeEnv(os.Environ(), envAdditionsForClaude(b.auth))
	toolCmd.Env, _ = execenv.PrependPATH(toolCmd.Env, execenv.DefaultExtraPathDirs())

	toolCmd.Stdin = stringsReader(prompt)

	stdout, err := toolCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := toolCmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := toolCmd.Start(); err != nil {
		if isExecutableNotFound(err) {
			return "", fmt.Errorf("start: %w (hint: ensure %q is on PATH; binary install often uses ~/.local/bin, or set config paths.claude / --claude-path)", err, cmdPath)
		}
		return "", fmt.Errorf("start: %w", err)
	}

	var (
		outMu sync.Mutex
		out   string
	)
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) == 0 && err != nil {
				return
			}
			line = bytesTrimSpace(line)
			if len(line) == 0 {
				if err != nil {
					return
				}
				continue
			}
			text := parseClaudeStreamJSONLine(line)
			if strings.TrimSpace(text) != "" {
				outMu.Lock()
				out = text
				outMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	var stderrBuf strings.Builder
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, 32*1024))
	}()

	waitErr := toolCmd.Wait()
	wg.Wait()

	outMu.Lock()
	final := strings.TrimSpace(out)
	outMu.Unlock()

	if waitErr != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return "", fmt.Errorf("claude error: %s", msg)
	}
	if final == "" {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg != "" {
			return "", fmt.Errorf("claude empty output: %s", msg)
		}
		return "", errors.New("claude empty output")
	}
	return final, nil
}

type CodexCLIBackend struct {
	cfg  config.Config
	auth *auth.Store
}

func isExecutableNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.Error
	if errors.As(err, &ee) && errors.Is(ee.Err, exec.ErrNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file or directory")
}

func NewCodexCLIBackend(cfg config.Config, authStore *auth.Store) Backend {
	return &CodexCLIBackend{cfg: cfg, auth: authStore}
}

func (b *CodexCLIBackend) Name() string { return "codex-cli" }

func (b *CodexCLIBackend) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := withDefaultTimeout(ctx, 60*time.Second)
	defer cancel()

	cmdPath := strings.TrimSpace(b.cfg.Paths.Codex)
	if cmdPath == "" {
		cmdPath = "codex"
	}

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"-C", b.cfg.Paths.DataDir,
		"--json",
		"-",
	}
	args = withCodexDefaults(args, b.auth)

	toolCmd := exec.CommandContext(ctx, cmdPath, args...)
	toolCmd.Dir = b.cfg.Paths.DataDir
	toolCmd.Env = mergeEnv(os.Environ(), envAdditionsForCodex(b.auth))
	toolCmd.Env, _ = execenv.PrependPATH(toolCmd.Env, execenv.DefaultExtraPathDirs())
	toolCmd.Stdin = stringsReader(prompt)

	stdout, err := toolCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := toolCmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := toolCmd.Start(); err != nil {
		if isExecutableNotFound(err) {
			return "", fmt.Errorf("start: %w (hint: ensure %q is on PATH, or set config paths.codex / --codex-path)", err, cmdPath)
		}
		return "", fmt.Errorf("start: %w", err)
	}

	var (
		outMu sync.Mutex
		out   string
	)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) == 0 && err != nil {
				return
			}
			line = bytesTrimSpace(line)
			if len(line) == 0 {
				if err != nil {
					return
				}
				continue
			}
			text := parseCodexJSONLine(line)
			if strings.TrimSpace(text) != "" {
				outMu.Lock()
				out = text
				outMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	var stderrBuf strings.Builder
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, 32*1024))
	}()

	waitErr := toolCmd.Wait()
	wg.Wait()

	outMu.Lock()
	final := strings.TrimSpace(out)
	outMu.Unlock()

	if waitErr != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return "", fmt.Errorf("codex error: %s", msg)
	}
	if final == "" {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg != "" {
			return "", fmt.Errorf("codex empty output: %s", msg)
		}
		return "", errors.New("codex empty output")
	}
	return final, nil
}

func envAdditionsForClaude(store *auth.Store) map[string]string {
	if store == nil {
		return nil
	}
	secrets := store.Get()
	add := map[string]string{}
	if strings.TrimSpace(secrets.AnthropicBaseURL) != "" {
		add["ANTHROPIC_BASE_URL"] = strings.TrimSpace(secrets.AnthropicBaseURL)
	}
	if strings.TrimSpace(secrets.AnthropicAPIKey) != "" {
		add["ANTHROPIC_API_KEY"] = strings.TrimSpace(secrets.AnthropicAPIKey)
	}
	if strings.TrimSpace(secrets.AnthropicAuthToken) != "" {
		add["ANTHROPIC_AUTH_TOKEN"] = strings.TrimSpace(secrets.AnthropicAuthToken)
	}
	if strings.TrimSpace(secrets.AnthropicModel) != "" {
		add["ANTHROPIC_MODEL"] = strings.TrimSpace(secrets.AnthropicModel)
	}
	if strings.TrimSpace(secrets.AnthropicSmallFastModel) != "" {
		add["ANTHROPIC_SMALL_FAST_MODEL"] = strings.TrimSpace(secrets.AnthropicSmallFastModel)
	}
	return add
}

func envAdditionsForCodex(store *auth.Store) map[string]string {
	if store == nil {
		return nil
	}
	secrets := store.Get()
	add := map[string]string{}
	if strings.TrimSpace(secrets.OpenAIAPIKey) != "" {
		add["OPENAI_API_KEY"] = strings.TrimSpace(secrets.OpenAIAPIKey)
	}
	return add
}

func withCodexDefaults(args []string, store *auth.Store) []string {
	model := "gpt-5.2"
	effort := "xhigh"
	if store != nil {
		secrets := store.Get()
		if strings.TrimSpace(secrets.CodexModel) != "" {
			model = strings.TrimSpace(secrets.CodexModel)
		}
		if strings.TrimSpace(secrets.CodexReasoningEffort) != "" {
			effort = strings.TrimSpace(secrets.CodexReasoningEffort)
		}
	}

	out := append([]string{}, args...)
	if !hasAnyFlag(out, "-m", "--model") && strings.TrimSpace(model) != "" {
		out = append(out, "-m", strings.TrimSpace(model))
	}
	if strings.TrimSpace(effort) != "" {
		out = append(out, "-c", fmt.Sprintf("model_reasoning_effort=%q", strings.TrimSpace(effort)))
	}
	return out
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func mergeEnv(base []string, additions map[string]string) []string {
	if len(additions) == 0 {
		return base
	}
	out := append([]string{}, base...)

	index := make(map[string]int, len(out))
	valueEmpty := make(map[string]bool, len(out))
	for i, kv := range out {
		j := strings.IndexByte(kv, '=')
		if j <= 0 {
			continue
		}
		k := kv[:j]
		v := kv[j+1:]
		if runtime.GOOS == "windows" {
			k = strings.ToUpper(k)
		}
		if _, ok := index[k]; ok {
			continue
		}
		index[k] = i
		valueEmpty[k] = strings.TrimSpace(v) == ""
	}

	for k, v := range additions {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		check := k
		if runtime.GOOS == "windows" {
			check = strings.ToUpper(check)
		}
		if i, ok := index[check]; ok {
			out[i] = k + "=" + v
			valueEmpty[check] = false
			continue
		}
		index[check] = len(out)
		out = append(out, k+"="+v)
	}
	return out
}

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func parseClaudeStreamJSONLine(line []byte) string {
	var evt struct {
		Type    string `json:"type"`
		Result  string `json:"result,omitempty"`
		Content string `json:"content,omitempty"`
		Role    string `json:"role,omitempty"`
		Delta   *bool  `json:"delta,omitempty"`
	}
	if err := json.Unmarshal(line, &evt); err != nil {
		return ""
	}
	if strings.TrimSpace(evt.Result) != "" {
		return evt.Result
	}
	if evt.Role == "assistant" && strings.TrimSpace(evt.Content) != "" {
		return evt.Content
	}
	return ""
}

func parseCodexJSONLine(line []byte) string {
	var evt struct {
		Type string          `json:"type"`
		Item json.RawMessage `json:"item,omitempty"`
	}
	if err := json.Unmarshal(line, &evt); err != nil {
		return ""
	}
	if evt.Type != "item.completed" || len(evt.Item) == 0 {
		return ""
	}
	var item struct {
		Type string      `json:"type"`
		Text interface{} `json:"text"`
	}
	if err := json.Unmarshal(evt.Item, &item); err != nil {
		return ""
	}
	if item.Type != "agent_message" {
		return ""
	}
	return normalizeCodexText(item.Text)
}

func normalizeCodexText(text interface{}) string {
	switch v := text.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if s, ok := item.(string); ok {
				sb.WriteString(s)
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), d)
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

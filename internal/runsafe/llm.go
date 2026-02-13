package runsafe

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	// install intent can unlock more permissive execution when the user enables install unlock,
	// so we require a higher confidence threshold than other intents.
	minInstallLLMConfidence = 0.8
)

type LLMBackend interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

type llmDecision struct {
	Intent     string   `json:"intent"`
	Confidence float64  `json:"confidence,omitempty"`
	Signals    []string `json:"signals,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type ClassifyOptions struct {
	LLM         LLMBackend
	LLMTimeout  time.Duration
	MinLLMScore float64
}

func ClassifyPrompt(ctx context.Context, prompt string, opts ClassifyOptions) Decision {
	det := ClassifyPromptDeterministic(prompt)

	llm := opts.LLM
	if llm == nil {
		return det
	}

	timeout := opts.LLMTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := llm.Complete(ctx, buildLLMClassifyPrompt(prompt))
	if err != nil {
		return det
	}
	parsed, ok := parseLLMDecision(raw)
	if !ok {
		return det
	}

	intent := normalizeIntent(parsed.Intent)
	if intent == "" {
		return det
	}

	min := opts.MinLLMScore
	if min <= 0 {
		min = 0.65
	}
	threshold := min
	if intent == IntentInstall && threshold < minInstallLLMConfidence {
		threshold = minInstallLLMConfidence
	}
	if parsed.Confidence <= 0 || parsed.Confidence < threshold {
		// Preserve a conservative default if the LLM is uncertain.
		det.Signals = dedupeStrings(append(det.Signals, "llm:reject"))
		if strings.TrimSpace(det.Reason) == "" {
			det.Reason = "llm confidence below threshold"
		}
		return det
	}

	out := det
	out.Intent = intent
	out.Confidence = parsed.Confidence
	out.Signals = dedupeStrings(append(parsed.Signals, "llm"))
	if strings.TrimSpace(parsed.Reason) != "" {
		out.Reason = strings.TrimSpace(parsed.Reason)
	}
	return out
}

func buildLLMClassifyPrompt(userPrompt string) string {
	userPrompt = strings.TrimSpace(userPrompt)
	return strings.TrimSpace(`你是一个本地 Agent 任务路由助手。你的任务是分析用户的输入，将其归类为以下四种意图（Intent）之一。

### 意图定义：
1. **search-browse (搜索浏览)**:
   - 当用户需要获取实时信息、查询文档、查找某个库的用法、或者询问当前不知道的事实。
   - 关键词：搜索、查询、找一下、google、ddg。
2. **install (环境安装)**:
   - 只有在用户明确要求安装依赖、配置环境、下载包或运行包管理器（pip, npm, cargo 等）时选择此项。
   - 关键词：安装、install、setup、pip install。
3. **analyze (代码分析)**:
   - 当用户要求解释代码、寻找 bug、阅读文件内容或理解现有逻辑，而不需要修改代码时。
   - 关键词：解释、分析、读一下、这是什么意思、debug。
4. **code (编写代码)**:
   - 当用户要求编写新代码、重构、修改现有代码或生成脚本时。
   - **默认规则**：如果意图模棱两可，或者涉及执行具体操作，请默认归类为 "code"。

### 输出格式规则：
请严格遵循JSON格式输出，不要包含任何其他开场白或结束语：
{"intent":"analyze|code|search-browse|install","confidence":0.0-1.0,"signals":["..."],"reason":"..."}

User prompt:
` + "\n" + userPrompt + "\n")
}

func parseLLMDecision(raw string) (llmDecision, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if s == "" {
		return llmDecision{}, false
	}
	var out llmDecision
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return llmDecision{}, false
	}
	return out, true
}

func normalizeIntent(raw string) Intent {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case string(IntentAnalyze):
		return IntentAnalyze
	case string(IntentCode):
		return IntentCode
	case string(IntentSearchBrowse):
		return IntentSearchBrowse
	case string(IntentInstall):
		return IntentInstall
	default:
		return ""
	}
}

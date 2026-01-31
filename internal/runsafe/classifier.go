package runsafe

import (
	"regexp"
	"strings"
)

var (
	reNpmInstall   = regexp.MustCompile(`(?i)\b(npm|pnpm|yarn)\s+(install|add)\b`)
	rePipInstall   = regexp.MustCompile(`(?i)\b(pip|pip3)\s+install\b`)
	rePoetryAdd    = regexp.MustCompile(`(?i)\bpoetry\s+add\b`)
	reCondaInstall = regexp.MustCompile(`(?i)\bconda\s+install\b`)
	reBrewInstall  = regexp.MustCompile(`(?i)\bbrew\s+install\b`)
	reAptInstall   = regexp.MustCompile(`(?i)\b(apt|apt-get)\s+install\b`)
	reDnfInstall   = regexp.MustCompile(`(?i)\bdnf\s+install\b`)
	rePacmanInstall = regexp.MustCompile(`(?i)\bpacman\s+-S\b`)
	reGoGet        = regexp.MustCompile(`(?i)\bgo\s+(get|install)\b`)
	reCargoInstall = regexp.MustCompile(`(?i)\bcargo\s+(install|add)\b`)
	reComposerReq  = regexp.MustCompile(`(?i)\bcomposer\s+require\b`)
)

func ClassifyPromptDeterministic(prompt string) Decision {
	raw := strings.TrimSpace(prompt)
	lower := strings.ToLower(raw)
	if raw == "" {
		return Decision{Intent: IntentCode, Confidence: 0.2, Reason: "empty prompt"}
	}

	// Install / dependency setup signals (high-risk category).
	if looksLikeInstall(lower) {
		return Decision{
			Intent:     IntentInstall,
			Confidence: 0.95,
			Signals:    []string{"install"},
			Reason:     "detected install/dependency keywords",
		}
	}

	// Explicit search/browse signals.
	if looksLikeSearchBrowse(lower) {
		return Decision{
			Intent:     IntentSearchBrowse,
			Confidence: 0.85,
			Signals:    []string{"search"},
			Reason:     "detected search/browse keywords",
		}
	}

	// Analysis-like phrasing (no strong "modify code" signals).
	if looksLikeAnalyze(lower) && !looksLikeCodeChange(lower) {
		return Decision{
			Intent:     IntentAnalyze,
			Confidence: 0.7,
			Signals:    []string{"analyze"},
			Reason:     "detected analysis/explanation keywords",
		}
	}

	// Default to code.
	return Decision{
		Intent:     IntentCode,
		Confidence: 0.55,
		Signals:    []string{"default"},
		Reason:     "default to code",
	}
}

func looksLikeInstall(lower string) bool {
	if strings.Contains(lower, "requirements.txt") || strings.Contains(lower, "package-lock.json") || strings.Contains(lower, "pnpm-lock.yaml") || strings.Contains(lower, "yarn.lock") {
		return true
	}
	if strings.Contains(lower, "安装依赖") || strings.Contains(lower, "装依赖") || strings.Contains(lower, "安装包") {
		return true
	}
	if strings.Contains(lower, "install dependencies") || strings.Contains(lower, "install dep") {
		return true
	}
	return reNpmInstall.MatchString(lower) ||
		rePipInstall.MatchString(lower) ||
		rePoetryAdd.MatchString(lower) ||
		reCondaInstall.MatchString(lower) ||
		reBrewInstall.MatchString(lower) ||
		reAptInstall.MatchString(lower) ||
		reDnfInstall.MatchString(lower) ||
		rePacmanInstall.MatchString(lower) ||
		reGoGet.MatchString(lower) ||
		reCargoInstall.MatchString(lower) ||
		reComposerReq.MatchString(lower)
}

func looksLikeSearchBrowse(lower string) bool {
	if strings.Contains(lower, "search-browse") {
		return true
	}
	if strings.Contains(lower, "搜索") || strings.Contains(lower, "查一下") || strings.Contains(lower, "查找") || strings.Contains(lower, "帮我搜") {
		return true
	}
	if strings.Contains(lower, "官网") || strings.Contains(lower, "文档") || strings.Contains(lower, "release note") || strings.Contains(lower, "release notes") || strings.Contains(lower, "changelog") {
		return true
	}
	if strings.Contains(lower, "look up") || strings.Contains(lower, "browse") || strings.Contains(lower, "search ") {
		return true
	}
	if strings.Contains(lower, "最新") || strings.Contains(lower, "today") || strings.Contains(lower, "recent") || strings.Contains(lower, "most recent") {
		return true
	}
	return false
}

func looksLikeAnalyze(lower string) bool {
	if strings.Contains(lower, "分析") || strings.Contains(lower, "总结") || strings.Contains(lower, "解释") || strings.Contains(lower, "review") || strings.Contains(lower, "评审") {
		return true
	}
	if strings.Contains(lower, "why") || strings.Contains(lower, "原因") || strings.Contains(lower, "怎么回事") {
		return true
	}
	return false
}

func looksLikeCodeChange(lower string) bool {
	if strings.Contains(lower, "修复") || strings.Contains(lower, "实现") || strings.Contains(lower, "重构") || strings.Contains(lower, "加一个") || strings.Contains(lower, "新增") {
		return true
	}
	if strings.Contains(lower, "fix ") || strings.Contains(lower, "implement ") || strings.Contains(lower, "refactor") {
		return true
	}
	return false
}


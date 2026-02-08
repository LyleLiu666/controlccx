package tasks

import "strings"

const (
	maxStderrScoreLines  = 10
	maxKeywordScoreHits  = 10
	blockedScoreBonus    = 3
	stderrScorePerLine   = 2
	nonZeroExitScore     = 2
	keywordScorePerMatch = 1
)

var keywordNeedles = []string{
	"error",
	"panic",
	"failed",
}

func ComputeScore(status Status, stderrCount, keywordCount int, exitCode *int) int {
	score := 0
	if status == StatusBlocked || status == StatusAwaitingApproval {
		score += blockedScoreBonus
	}
	if stderrCount > maxStderrScoreLines {
		stderrCount = maxStderrScoreLines
	}
	score += stderrCount * stderrScorePerLine
	if status == StatusFailed && exitCode != nil && *exitCode != 0 {
		score += nonZeroExitScore
	}
	if keywordCount > maxKeywordScoreHits {
		keywordCount = maxKeywordScoreHits
	}
	score += keywordCount * keywordScorePerMatch
	return score
}

func CountKeywordHits(s string) int {
	s = strings.ToLower(s)
	hits := 0
	for _, needle := range keywordNeedles {
		if strings.Contains(s, needle) {
			hits++
		}
	}
	return hits
}

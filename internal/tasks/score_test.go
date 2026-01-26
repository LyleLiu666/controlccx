package tasks

import "testing"

func TestComputeScore(t *testing.T) {
	exit0 := 0
	exit1 := 1

	t.Run("baseline", func(t *testing.T) {
		if got := ComputeScore(StatusRunning, 0, 0, nil); got != 0 {
			t.Fatalf("score=%d, want 0", got)
		}
	})

	t.Run("blocked bonus", func(t *testing.T) {
		if got := ComputeScore(StatusBlocked, 0, 0, nil); got != blockedScoreBonus {
			t.Fatalf("score=%d, want %d", got, blockedScoreBonus)
		}
	})

	t.Run("stderr cap", func(t *testing.T) {
		got := ComputeScore(StatusRunning, maxStderrScoreLines+5, 0, nil)
		want := maxStderrScoreLines * stderrScorePerLine
		if got != want {
			t.Fatalf("score=%d, want %d", got, want)
		}
	})

	t.Run("keywords cap", func(t *testing.T) {
		got := ComputeScore(StatusRunning, 0, maxKeywordScoreHits+5, nil)
		want := maxKeywordScoreHits * keywordScorePerMatch
		if got != want {
			t.Fatalf("score=%d, want %d", got, want)
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		got := ComputeScore(StatusFailed, 0, 0, &exit1)
		want := nonZeroExitScore
		if got != want {
			t.Fatalf("score=%d, want %d", got, want)
		}
	})

	t.Run("zero exit does not add", func(t *testing.T) {
		if got := ComputeScore(StatusSucceeded, 0, 0, &exit0); got != 0 {
			t.Fatalf("score=%d, want 0", got)
		}
	})
}

func TestCountKeywordHits(t *testing.T) {
	if got := CountKeywordHits("ERROR failed Panic"); got != 3 {
		t.Fatalf("hits=%d, want 3", got)
	}
	if got := CountKeywordHits("all good"); got != 0 {
		t.Fatalf("hits=%d, want 0", got)
	}
}


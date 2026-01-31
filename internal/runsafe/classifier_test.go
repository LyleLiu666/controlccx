package runsafe

import "testing"

func TestClassifyPromptDeterministic(t *testing.T) {
	t.Run("install", func(t *testing.T) {
		got := ClassifyPromptDeterministic("请在项目里运行 npm install 并解决依赖冲突")
		if got.Intent != IntentInstall {
			t.Fatalf("intent=%q, want %q", got.Intent, IntentInstall)
		}
	})

	t.Run("search-browse", func(t *testing.T) {
		got := ClassifyPromptDeterministic("帮我搜索一下 Codex CLI 最新版本的参数说明")
		if got.Intent != IntentSearchBrowse {
			t.Fatalf("intent=%q, want %q", got.Intent, IntentSearchBrowse)
		}
	})

	t.Run("analyze", func(t *testing.T) {
		got := ClassifyPromptDeterministic("请总结这段代码在做什么，并指出风险点")
		if got.Intent != IntentAnalyze {
			t.Fatalf("intent=%q, want %q", got.Intent, IntentAnalyze)
		}
	})

	t.Run("code default", func(t *testing.T) {
		got := ClassifyPromptDeterministic("修复这个 bug：启动时报错 nil pointer")
		if got.Intent != IntentCode {
			t.Fatalf("intent=%q, want %q", got.Intent, IntentCode)
		}
	})
}


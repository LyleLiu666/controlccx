package worker

import (
	"strings"
	"testing"

	"controlccx/internal/tasks"
)

func TestSessionIDToPersist_NewMode_UsesObserved(t *testing.T) {
	task := tasks.Task{Mode: tasks.ModeNew, SessionID: ""}
	sid, warn := sessionIDToPersist(task, "sess-new")
	if sid != "sess-new" {
		t.Fatalf("sid=%q want %q", sid, "sess-new")
	}
	if warn != "" {
		t.Fatalf("unexpected warn=%q", warn)
	}
}

func TestSessionIDToPersist_ResumeMode_SameSessionID(t *testing.T) {
	task := tasks.Task{Mode: tasks.ModeResume, SessionID: "sess-1"}
	sid, warn := sessionIDToPersist(task, "sess-1")
	if sid != "sess-1" {
		t.Fatalf("sid=%q want %q", sid, "sess-1")
	}
	if warn != "" {
		t.Fatalf("unexpected warn=%q", warn)
	}
}

func TestSessionIDToPersist_ResumeMode_Mismatch_KeepsRequested(t *testing.T) {
	task := tasks.Task{Mode: tasks.ModeResume, SessionID: "sess-1"}
	sid, warn := sessionIDToPersist(task, "sess-NEW")
	if sid != "" {
		t.Fatalf("sid=%q want empty (keep requested)", sid)
	}
	if warn == "" || !strings.Contains(warn, "requested") || !strings.Contains(warn, "observed") {
		t.Fatalf("warn=%q, want a helpful warning", warn)
	}
}

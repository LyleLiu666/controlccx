package tasks

import "testing"

func TestConversationAnchorForTask(t *testing.T) {
	t.Run("conversation scoped anchor", func(t *testing.T) {
		task := Task{
			ID:             "task-1",
			SessionID:      "sess-1",
			ConversationID: "  conv-1  ",
		}
		if got := ConversationAnchorForTask(task); got != "c:conv-1" {
			t.Fatalf("anchor=%q, want %q", got, "c:conv-1")
		}
		if got := SessionKeyForTask(task); got != "c:conv-1" {
			t.Fatalf("session key=%q, want %q", got, "c:conv-1")
		}
	})

	t.Run("legacy fallback when conversation id missing", func(t *testing.T) {
		task := Task{
			ID:             "task-2",
			SessionID:      "sess-2",
			ConversationID: "",
		}
		if got := ConversationAnchorForTask(task); got != "" {
			t.Fatalf("anchor=%q, want empty", got)
		}
		if got := SessionKeyForTask(task); got != "s:sess-2" {
			t.Fatalf("session key=%q, want %q", got, "s:sess-2")
		}
	})
}


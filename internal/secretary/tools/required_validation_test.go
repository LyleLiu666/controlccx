package tools

import (
	"strings"
	"testing"
)

func TestValidateRequired_RequiredFields(t *testing.T) {
	err := validateRequired(map[string]string{"a": "1"}, []string{"a", "b"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := strings.ToLower(err.Error()); !strings.Contains(got, "b") {
		t.Fatalf("error=%q, want mention missing key b", err.Error())
	}
}

func TestValidateRequired_RequiredFields_Multiple(t *testing.T) {
	err := validateRequired(map[string]string{}, []string{"a", "b"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("error=%q, want mention keys a and b", got)
	}
	if !strings.Contains(got, "missing required fields") {
		t.Fatalf("error=%q, want plural required fields message", got)
	}
}

func TestValidateRequired_TrimsWhitespace(t *testing.T) {
	err := validateRequired(map[string]string{"a": "   ", "b": "ok"}, []string{" a ", "b"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := strings.ToLower(err.Error()); !strings.Contains(got, "a") {
		t.Fatalf("error=%q, want mention missing key a", err.Error())
	}
}

func TestValidateRequired_AnyOfRequired(t *testing.T) {
	err := validateRequired(map[string]string{"a": ""}, nil, [][]string{{"a", "b"}})
	if err == nil {
		t.Fatalf("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("error=%q, want mention group keys a and b", got)
	}
	if !strings.Contains(strings.ToLower(got), "one of") {
		t.Fatalf("error=%q, want one-of message", got)
	}

	err = validateRequired(map[string]string{"b": "1"}, nil, [][]string{{"a", "b"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequired_MultipleAnyOfGroups(t *testing.T) {
	err := validateRequired(map[string]string{"a": "1"}, nil, [][]string{{"a", "b"}, {"c", "d"}})
	if err == nil {
		t.Fatalf("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "c") || !strings.Contains(got, "d") {
		t.Fatalf("error=%q, want mention unsatisfied group keys c and d", got)
	}
}

func TestValidateRequired_EmptyOK(t *testing.T) {
	if err := validateRequired(nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequired_ConfirmUsesTrueHint(t *testing.T) {
	err := validateRequired(map[string]string{"task_id": "t1"}, []string{"confirm"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := strings.ToLower(err.Error()); !strings.Contains(got, "confirm=true") {
		t.Fatalf("error=%q, want confirm=true hint", err.Error())
	}
}

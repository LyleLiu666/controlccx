package tools

import (
	"strings"
	"testing"
)

func TestDescriptors_ParamListsAreWellFormed(t *testing.T) {
	for _, d := range Descriptors() {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			t.Fatalf("found empty descriptor name")
		}

		if len(d.Params) == 0 {
			if len(d.Required) > 0 || len(d.AnyOfRequired) > 0 {
				t.Fatalf("descriptor %q has required fields but no params", name)
			}
			continue
		}

		seen := make(map[string]struct{}, len(d.Params))
		for _, p := range d.Params {
			p = strings.TrimSpace(p)
			if p == "" {
				t.Fatalf("descriptor %q has empty param", name)
			}
			if _, ok := seen[p]; ok {
				t.Fatalf("descriptor %q has duplicate param %q", name, p)
			}
			seen[p] = struct{}{}
		}

		for _, r := range d.Required {
			r = strings.TrimSpace(r)
			if r == "" {
				t.Fatalf("descriptor %q has empty required param", name)
			}
			if _, ok := seen[r]; !ok {
				t.Fatalf("descriptor %q required param %q not in Params", name, r)
			}
		}
		for _, group := range d.AnyOfRequired {
			if len(group) == 0 {
				t.Fatalf("descriptor %q has empty AnyOfRequired group", name)
			}
			for _, p := range group {
				p = strings.TrimSpace(p)
				if p == "" {
					t.Fatalf("descriptor %q has empty AnyOfRequired param", name)
				}
				if _, ok := seen[p]; !ok {
					t.Fatalf("descriptor %q AnyOfRequired param %q not in Params", name, p)
				}
			}
		}
	}
}

func TestDescriptors_P0ToolsHaveParams(t *testing.T) {
	p0 := []string{
		"task_continue_submit",
		"task_preempt_continue_submit",
		"task_resume_submit",
		"task_rehydrate_submit",
		"task_new_submit",
		"scheduler_create",
	}

	got := make(map[string]Descriptor, 64)
	for _, d := range Descriptors() {
		got[d.Name] = d
	}

	for _, name := range p0 {
		d, ok := got[name]
		if !ok {
			t.Fatalf("missing descriptor for %q", name)
		}
		if len(d.Params) == 0 {
			t.Fatalf("expected P0 tool %q to have Params", name)
		}
	}
}

func TestDescriptors_AllParamToolsHaveParams(t *testing.T) {
	noParams := map[string]struct{}{
		"system_info":    {},
		"fs_roots":       {},
		"fs_pwd":         {},
		"scheduler_list": {},
	}
	for _, d := range Descriptors() {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			t.Fatalf("found empty descriptor name")
		}
		_, expectNoParams := noParams[name]
		if expectNoParams {
			if len(d.Params) != 0 || len(d.Required) != 0 || len(d.AnyOfRequired) != 0 {
				t.Fatalf("expected %q to have no param metadata, got params=%v required=%v anyof=%v", name, d.Params, d.Required, d.AnyOfRequired)
			}
			continue
		}
		if len(d.Params) == 0 {
			t.Fatalf("expected %q to have Params", name)
		}
	}
}

func TestDescriptors_P0RunOptsToolsContainAllRunOptsParams(t *testing.T) {
	runOptsTools := []string{
		"task_continue_submit",
		"task_preempt_continue_submit",
		"task_resume_submit",
		"task_rehydrate_submit",
		"task_new_submit",
	}

	got := make(map[string]Descriptor, 64)
	for _, d := range Descriptors() {
		got[d.Name] = d
	}

	for _, name := range runOptsTools {
		d := got[name]
		if len(d.Params) == 0 {
			t.Fatalf("expected tool %q to have Params", name)
		}
		set := make(map[string]struct{}, len(d.Params))
		for _, p := range d.Params {
			set[strings.TrimSpace(p)] = struct{}{}
		}
		for _, key := range RunOptsParams {
			if _, ok := set[key]; !ok {
				t.Fatalf("expected tool %q Params to include runopts key %q", name, key)
			}
		}
	}
}

func TestDescriptors_SchedulerCreateAnyOfRequired(t *testing.T) {
	var d Descriptor
	var ok bool
	for _, x := range Descriptors() {
		if x.Name == "scheduler_create" {
			d = x
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("missing descriptor for scheduler_create")
	}
	if len(d.Params) == 0 {
		t.Fatalf("expected scheduler_create to have Params")
	}

	requireSet := make(map[string]struct{}, len(d.Required))
	for _, p := range d.Required {
		requireSet[strings.TrimSpace(p)] = struct{}{}
	}
	if _, ok := requireSet["tool_fields_json"]; !ok {
		t.Fatalf("expected scheduler_create Required to contain tool_fields_json")
	}

	wantAnyOf := map[string]struct{}{
		"tool_name":        {},
		"target_tool_name": {},
		"name":             {},
	}
	found := false
	for _, group := range d.AnyOfRequired {
		if len(group) != len(wantAnyOf) {
			continue
		}
		groupSet := make(map[string]struct{}, len(group))
		for _, p := range group {
			groupSet[strings.TrimSpace(p)] = struct{}{}
		}
		match := true
		for k := range wantAnyOf {
			if _, ok := groupSet[k]; !ok {
				match = false
				break
			}
		}
		if match {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scheduler_create AnyOfRequired to contain {tool_name,target_tool_name,name}, got: %#v", d.AnyOfRequired)
	}
}

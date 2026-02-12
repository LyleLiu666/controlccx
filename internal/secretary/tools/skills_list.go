package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/skills"
)

type skillsListTool struct{}

func (skillsListTool) Name() string { return "skills_list" }

func (skillsListTool) DescriptionZH() string {
	return "列出本机技能（skills）及其在各目标（codex/claude_code/cursor/opencode/antigravity）的可用状态。参数：target（可选，过滤指定目标，如 codex 或 claude-code）、q（可选，名称模糊匹配）、only_enabled（可选，1/true 仅返回在目标里可用的技能）、include_paths（可选，1/true 时包含本机路径细节；默认不返回路径，避免输出过大）、limit（可选，默认200，最大500）、offset（可选，默认0）。"
}

func normalizeSkillsTarget(raw string) skills.Target {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return ""
	case "claude-code", "claude_code", "claude":
		return skills.TargetClaudeCode
	case "codex":
		return skills.TargetCodex
	case "cursor":
		return skills.TargetCursor
	case "antigravity":
		return skills.TargetAntigravity
	case "opencode":
		return skills.TargetOpencode
	default:
		return ""
	}
}

type skillsListTarget struct {
	Target     skills.Target      `json:"target"`
	Status     skills.EntryStatus `json:"status"`
	Root       string             `json:"root,omitempty"`
	LinkTarget string             `json:"link_target,omitempty"`
	Note       string             `json:"note,omitempty"`
}

type skillsListEntry struct {
	Name    string             `json:"name"`
	RepoKey string             `json:"repo_key,omitempty"`
	Targets []skillsListTarget `json:"targets,omitempty"`
	Enabled bool               `json:"enabled"`

	Sources []string `json:"sources,omitempty"`
	Source  string   `json:"source,omitempty"`
}

func targetStateEnabled(status skills.EntryStatus) bool {
	switch status {
	case skills.StatusLinked, skills.StatusCopied, skills.StatusPresent, skills.StatusExternal:
		return true
	default:
		return false
	}
}

func skillEnabledAny(item skills.Skill) bool {
	for _, st := range item.Targets {
		if targetStateEnabled(st.Status) {
			return true
		}
	}
	return false
}

func skillFilterTargets(item skills.Skill, target skills.Target) skills.Skill {
	if strings.TrimSpace(string(target)) == "" {
		return item
	}
	out := item
	out.Targets = nil
	for _, st := range item.Targets {
		if st.Target == target {
			out.Targets = append(out.Targets, st)
		}
	}
	return out
}

func (skillsListTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Skills == nil {
		return nil, errors.New("skills service not configured")
	}

	out, err := deps.Skills.List(ctx)
	if err != nil {
		return nil, err
	}

	q := strings.TrimSpace(call.Fields["q"])
	needle := strings.ToLower(q)

	targetRaw := strings.TrimSpace(call.Fields["target"])
	target := normalizeSkillsTarget(targetRaw)
	if targetRaw != "" && target == "" {
		return nil, fmt.Errorf("unknown target %q", targetRaw)
	}
	onlyEnabled := parseBool(call.Fields["only_enabled"])
	includePaths := parseBool(call.Fields["include_paths"])

	filtered := make([]skillsListEntry, 0, len(out.Skills))
	for _, item := range out.Skills {
		if needle != "" && !strings.Contains(strings.ToLower(item.Name), needle) {
			continue
		}
		if target != "" {
			item = skillFilterTargets(item, target)
		}
		enabled := skillEnabledAny(item)
		if onlyEnabled && !enabled {
			continue
		}

		entry := skillsListEntry{
			Name:    item.Name,
			RepoKey: strings.TrimSpace(item.RepoKey),
			Enabled: enabled,
		}
		if includePaths {
			entry.Sources = item.Sources
			entry.Source = item.PreferredSource
		}
		for _, st := range item.Targets {
			ts := skillsListTarget{
				Target: st.Target,
				Status: st.Status,
			}
			if includePaths {
				ts.Root = st.Root
				ts.LinkTarget = st.LinkTarget
				ts.Note = st.Note
			}
			entry.Targets = append(entry.Targets, ts)
		}

		filtered = append(filtered, entry)
	}

	total := len(filtered)
	offset := parseInt(call.Fields["offset"], 0)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	limit := parseInt(call.Fields["limit"], 200)
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	res := map[string]any{
		"skills": page,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	}
	if q != "" {
		res["q"] = q
	}
	if target != "" {
		res["target"] = target
	}
	if onlyEnabled {
		res["only_enabled"] = true
	}
	if includePaths {
		res["include_paths"] = true
	}
	return res, nil
}

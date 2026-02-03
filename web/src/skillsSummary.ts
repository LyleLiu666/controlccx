import type { Skill } from "./types.ts";

export type SkillTarget = "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";

export type SkillsSummary = {
  target: SkillTarget;
  status:
    | "missing"
    | "linked"
    | "broken"
    | "present"
    | "copied"
    | "conflict"
    | "external"
    | "partial";
  canEnable: boolean;
  canDisable: boolean;
  enabled: boolean;
  detail: string;
};

export function summarizeSkillTarget(skill: Skill, target: SkillTarget): SkillsSummary {
  const states = (skill.targets ?? []).filter((s) => s.target === target);
  const detail = states
    .map((s) => `${s.root}: ${s.status}${s.note ? ` (${s.note})` : ""}`)
    .join("\n");

  const hasSource = !!(skill.source && skill.source.trim());
  const allStatuses = (skill.targets ?? []).map((s) => s.status);
  const hasBootstrapCandidate = allStatuses.some(
    (s) => s === "present" || s === "external" || s === "copied" || s === "linked",
  );
  const statuses = states.map((s) => s.status);
  const unique = Array.from(new Set(statuses));
  const status =
    unique.length === 1 && unique[0]
      ? (unique[0] as SkillsSummary["status"])
      : "partial";

  const anyManagedEnabled = statuses.some((s) => s === "linked" || s === "copied");
  const anyBroken = statuses.some((s) => s === "broken");
  const enabled = anyManagedEnabled;

  const hasUnmanagedBlocker = statuses.some(
    (s) => s === "present" || s === "conflict" || s === "external",
  );
  const canEnable = (hasSource || hasBootstrapCandidate) && !hasUnmanagedBlocker;
  const canDisable = !hasUnmanagedBlocker && (anyManagedEnabled || anyBroken);

  return { target, status, canEnable, canDisable, enabled, detail };
}

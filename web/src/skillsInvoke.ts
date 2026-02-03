import type { ToolDriver } from "./types";

function normalizeSkillName(name: string): string {
  return String(name ?? "").trim();
}

export function formatSkillToken(name: string, driver: ToolDriver): string {
  const skill = normalizeSkillName(name);
  if (!skill) return "";

  switch (driver) {
    case "claude-code":
      return `/${skill}`;
    case "codex":
      return `$${skill}`;
    default:
      return `$${skill}`;
  }
}


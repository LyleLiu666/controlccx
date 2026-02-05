import type { Skill, ToolDriver } from "./types.ts";
import { extractSkillTokenNames } from "./skillsInvoke.ts";
import { summarizeSkillTarget, type SkillTarget, type SkillsSummary } from "./skillsSummary.ts";

export type SkillMountConfirmItem = {
  name: string;
  status: SkillsSummary["status"];
  detail: string;
};

export type SkillMountPlan = {
  target: SkillTarget;
  items: SkillMountConfirmItem[];
  namesToMount: string[];
};

export function buildSkillMountPlan(opts: {
  driver: ToolDriver;
  prompt: string;
  skills: Skill[];
}): SkillMountPlan | null {
  const driver = opts.driver;
  const prompt = String(opts.prompt ?? "");
  const skills = Array.isArray(opts.skills) ? opts.skills : [];

  const target: SkillTarget | null =
    driver === "codex" ? "codex" : driver === "claude-code" ? "claude_code" : null;
  if (!target) return null;

  const names = extractSkillTokenNames(prompt);
  if (names.length === 0) return null;

  const byName = new Map<string, Skill>();
  for (const s of skills) byName.set(String(s.name ?? ""), s);

  const items: SkillMountConfirmItem[] = [];
  const namesToMount: string[] = [];
  for (const name of names) {
    const sk = byName.get(name);
    if (!sk) continue;
    const summary = summarizeSkillTarget(sk, target);
    if (!summary.canEnable || summary.enabled) continue;
    items.push({ name, status: summary.status, detail: summary.detail });
    namesToMount.push(name);
  }

  if (namesToMount.length === 0) return null;
  return { target, items, namesToMount };
}

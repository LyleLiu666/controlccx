import type { Skill } from "./types.ts";

export type SkillsRepoGroup = {
  key: string;
  label: string;
  ref: string;
  skills: Skill[];
};

export type SkillsRepoView = {
  items: Skill[];
  groups: SkillsRepoGroup[];
  groupedEmpty: boolean;
};

export function buildSkillsRepoView(opts: {
  skills: Skill[];
  q?: string;
  repo?: string;
  groupByRepo?: boolean;
}): SkillsRepoView {
  const skills = Array.isArray(opts.skills) ? opts.skills : [];
  const q = String(opts.q ?? "").trim().toLowerCase();
  const repo = String(opts.repo ?? "").trim();
  const groupByRepo = !!opts.groupByRepo;

  let filtered = skills.filter((s) => {
    if (repo && String(s.repo_key ?? "") !== repo) return false;
    if (!q) return true;
    const hay = [s.name, s.source, s.repo_label, s.repo_ref]
      .map((v) => String(v ?? "").toLowerCase())
      .join("\n");
    return hay.includes(q);
  });

  if (groupByRepo) {
    filtered = filtered.filter((s) => String(s.repo_key ?? "").trim() !== "");
  }

  filtered = filtered
    .slice()
    .sort((a, b) => String(a.name ?? "").localeCompare(String(b.name ?? "")));

  if (!groupByRepo) {
    return {
      items: filtered,
      groups: [],
      groupedEmpty: false,
    };
  }

  const byRepo = new Map<string, SkillsRepoGroup>();
  for (const s of filtered) {
    const key = String(s.repo_key ?? "").trim();
    if (!key) continue;
    const label = String(s.repo_label ?? "").trim() || key;
    const ref = String(s.repo_ref ?? "").trim();
    let group = byRepo.get(key);
    if (!group) {
      group = { key, label, ref, skills: [] };
      byRepo.set(key, group);
    }
    if (!group.ref && ref) group.ref = ref;
    group.skills.push(s);
  }

  const groups = Array.from(byRepo.values()).sort((a, b) => {
    const la = a.label.toLowerCase();
    const lb = b.label.toLowerCase();
    if (la === lb) return a.key.localeCompare(b.key);
    return la.localeCompare(lb);
  });

  for (const g of groups) {
    g.skills = g.skills
      .slice()
      .sort((a, b) => String(a.name ?? "").localeCompare(String(b.name ?? "")));
  }

  return {
    items: filtered,
    groups,
    groupedEmpty: filtered.length === 0,
  };
}

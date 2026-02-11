import { computed, ref, watch } from "vue";
import type { Skill, SkillsListResponse } from "../types";
import { fetchSkills, linkSkill, syncSkill, unlinkSkill } from "../api";

import type { SkillTarget } from "../skillsSummary";
import { summarizeSkillTarget } from "../skillsSummary";

const LS_KEY_SKILLS_REPO_FILTER = "controlccx.skills.repo_filter.v1";
const LS_KEY_SKILLS_GROUP_BY_REPO = "controlccx.skills.group_by_repo.v1";

function getLocalStorageSafe(): Storage | null {
  try {
    return window?.localStorage ?? null;
  } catch {
    return null;
  }
}

function loadString(key: string): string {
  const st = getLocalStorageSafe();
  if (!st) return "";
  try {
    return String(st.getItem(key) ?? "").trim();
  } catch {
    return "";
  }
}

function saveString(key: string, value: string) {
  const st = getLocalStorageSafe();
  if (!st) return;
  try {
    const v = String(value ?? "").trim();
    if (!v) st.removeItem(key);
    else st.setItem(key, v);
  } catch {
    // ignore
  }
}

function loadBool(key: string, fallback = false): boolean {
  const st = getLocalStorageSafe();
  if (!st) return fallback;
  try {
    const raw = String(st.getItem(key) ?? "").trim().toLowerCase();
    if (raw === "1" || raw === "true") return true;
    if (raw === "0" || raw === "false") return false;
  } catch {
    // ignore
  }
  return fallback;
}

function saveBool(key: string, value: boolean) {
  const st = getLocalStorageSafe();
  if (!st) return;
  try {
    st.setItem(key, value ? "1" : "0");
  } catch {
    // ignore
  }
}

export function useSkills() {
  const skillsOpen = ref(false);
  const skillsLoading = ref(false);
  const skillsError = ref("");
  const skillsFilter = ref("");
  const skillsRepoFilter = ref(loadString(LS_KEY_SKILLS_REPO_FILTER));
  const skillsGroupByRepo = ref(loadBool(LS_KEY_SKILLS_GROUP_BY_REPO, false));
  const skillsLimit = ref(200);
  const skillsOffset = ref(0);
  const skillsData = ref<SkillsListResponse | null>(null);
  const skillsActionBusy = ref<Map<string, boolean>>(new Map());

  const skillsTotal = computed(
    () => skillsData.value?.total ?? (skillsData.value?.skills.length ?? 0),
  );
  const skillsRangeLabel = computed(() => {
    const total = skillsTotal.value;
    if (!total) return "0";
    const pageLen = skillsData.value?.skills.length ?? 0;
    const from = Math.min(total, skillsOffset.value + 1);
    const to = Math.min(total, skillsOffset.value + pageLen);
    return `${from}-${to} / ${total}`;
  });
  const skillsCanPrev = computed(() => skillsOffset.value > 0);
  const skillsCanNext = computed(() => {
    const total = skillsTotal.value;
    if (!total) return false;
    const pageLen = skillsData.value?.skills.length ?? 0;
    return skillsOffset.value + pageLen < total;
  });

  async function refreshSkills() {
    skillsError.value = "";
    skillsLoading.value = true;
    try {
      skillsData.value = await fetchSkills({
        q: skillsFilter.value,
        repo: skillsRepoFilter.value,
        limit: skillsLimit.value,
        offset: skillsOffset.value,
      });
    } catch (e: any) {
      skillsError.value = e?.message ?? String(e);
    } finally {
      skillsLoading.value = false;
    }
  }

  async function openSkills() {
    skillsError.value = "";
    skillsOpen.value = true;
    await refreshSkills();
  }

  let skillsRefreshTimer: number | null = null;
  watch([skillsFilter, skillsRepoFilter, skillsLimit], () => {
    skillsOffset.value = 0;
    if (!skillsOpen.value) return;
    if (skillsRefreshTimer) window.clearTimeout(skillsRefreshTimer);
    skillsRefreshTimer = window.setTimeout(() => void refreshSkills(), 250);
  });
  watch(skillsRepoFilter, (value) => saveString(LS_KEY_SKILLS_REPO_FILTER, value));
  watch(skillsGroupByRepo, (value) => saveBool(LS_KEY_SKILLS_GROUP_BY_REPO, value));
  watch(skillsOpen, (open) => {
    if (!open && skillsRefreshTimer) {
      window.clearTimeout(skillsRefreshTimer);
      skillsRefreshTimer = null;
    }
  });

  async function skillsPrevPage() {
    if (skillsLoading.value) return;
    skillsOffset.value = Math.max(0, skillsOffset.value - skillsLimit.value);
    await refreshSkills();
  }

  async function skillsNextPage() {
    if (skillsLoading.value) return;
    skillsOffset.value = skillsOffset.value + skillsLimit.value;
    await refreshSkills();
  }

  function skillsKey(name: string, target: SkillTarget) {
    return `${target}:${name}`;
  }

  async function onSkillsToggle(
    name: string,
    target: SkillTarget,
    enable: boolean,
  ) {
    const key = skillsKey(name, target);
    skillsActionBusy.value.set(key, true);
    skillsActionBusy.value = new Map(skillsActionBusy.value);
    skillsError.value = "";
    try {
      if (enable) {
        await linkSkill({ name, target, auto_import: true, prefer_tool: "claude_code" });
      } else {
        await unlinkSkill({ name, target });
      }
      await refreshSkills();
    } catch (e: any) {
      skillsError.value = e?.message ?? String(e);
    } finally {
      skillsActionBusy.value.delete(key);
      skillsActionBusy.value = new Map(skillsActionBusy.value);
    }
  }

  async function onSkillsTakeover(name: string, target: SkillTarget) {
    const key = skillsKey(name, target);
    if (
      typeof window !== "undefined" &&
      !window.confirm(
        `将覆盖 ${target}:${name} 的现有条目，并替换为受控关联（旧目录会备份到 ~/.controlccx/skills_backups）。继续？`,
      )
    ) {
      return;
    }
    skillsActionBusy.value.set(key, true);
    skillsActionBusy.value = new Map(skillsActionBusy.value);
    skillsError.value = "";
    try {
      await syncSkill({ name, target, overwrite: true });
      await refreshSkills();
    } catch (e: any) {
      skillsError.value = e?.message ?? String(e);
    } finally {
      skillsActionBusy.value.delete(key);
      skillsActionBusy.value = new Map(skillsActionBusy.value);
    }
  }

  function skillBadgeClass(status: string) {
    switch (status) {
      case "linked":
      case "copied":
        return "ok";
      case "broken":
      case "conflict":
        return "warn";
      case "external":
      case "present":
        return "muted";
      case "missing":
        return "dim";
      case "partial":
        return "partial";
      default:
        return "dim";
    }
  }

  const skillsVisible = computed(() => skillsData.value?.skills ?? []);

  return {
    skillsOpen,
    skillsLoading,
    skillsError,
    skillsFilter,
    skillsRepoFilter,
    skillsGroupByRepo,
    skillsLimit,
    skillsOffset,
    skillsData,
    skillsActionBusy,

    skillsTotal,
    skillsRangeLabel,
    skillsCanPrev,
    skillsCanNext,
    skillsVisible,

    openSkills,
    refreshSkills,
    skillsPrevPage,
    skillsNextPage,
    skillsKey,
    onSkillsToggle,
    onSkillsTakeover,
    summarizeSkillTarget,
    skillBadgeClass,
  };
}

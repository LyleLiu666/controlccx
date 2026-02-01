import { computed, ref, watch } from "vue";
import type { Skill, SkillsListResponse } from "../types";
import { fetchSkills, linkSkill, unlinkSkill } from "../api";

import type { SkillTarget } from "../skillsSummary";
import { summarizeSkillTarget } from "../skillsSummary";

export function useSkills() {
  const skillsOpen = ref(false);
  const skillsLoading = ref(false);
  const skillsError = ref("");
  const skillsFilter = ref("");
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
  watch([skillsFilter, skillsLimit], () => {
    skillsOffset.value = 0;
    if (!skillsOpen.value) return;
    if (skillsRefreshTimer) window.clearTimeout(skillsRefreshTimer);
    skillsRefreshTimer = window.setTimeout(() => void refreshSkills(), 250);
  });
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
        await linkSkill({ name, target });
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
    summarizeSkillTarget,
    skillBadgeClass,
  };
}

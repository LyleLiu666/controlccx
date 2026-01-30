import { computed, ref } from "vue";
import type { SkillVersionsListResponse } from "../types";
import { createSkillVersion, deleteSkillVersion, fetchSkillVersions } from "../api";

export function useSkillVersions() {
  const skillsVersionsLoading = ref(false);
  const skillsVersionsError = ref("");
  const skillsVersionsData = ref<SkillVersionsListResponse | null>(null);

  const skillsVersionNewId = ref("");
  const skillsVersionNewNote = ref("");

  const skillsVersionsCreating = ref(false);
  const skillsVersionsDeleting = ref<Map<string, boolean>>(new Map());

  const skillsVersionsList = computed(() => skillsVersionsData.value?.versions ?? []);

  async function refreshSkillVersions() {
    skillsVersionsError.value = "";
    skillsVersionsLoading.value = true;
    try {
      skillsVersionsData.value = await fetchSkillVersions();
    } catch (e: any) {
      skillsVersionsError.value = e?.message ?? String(e);
    } finally {
      skillsVersionsLoading.value = false;
    }
  }

  async function createSkillVersionFromForm() {
    if (skillsVersionsCreating.value) return;
    skillsVersionsCreating.value = true;
    skillsVersionsError.value = "";
    try {
      await createSkillVersion({
        id: skillsVersionNewId.value.trim(),
        note: skillsVersionNewNote.value.trim(),
      });
      skillsVersionNewId.value = "";
      skillsVersionNewNote.value = "";
      await refreshSkillVersions();
    } catch (e: any) {
      skillsVersionsError.value = e?.message ?? String(e);
    } finally {
      skillsVersionsCreating.value = false;
    }
  }

  async function deleteSkillVersionByID(id: string) {
    id = (id ?? "").trim();
    if (!id) return;
    skillsVersionsDeleting.value.set(id, true);
    skillsVersionsDeleting.value = new Map(skillsVersionsDeleting.value);
    skillsVersionsError.value = "";
    try {
      await deleteSkillVersion({ id });
      await refreshSkillVersions();
    } catch (e: any) {
      skillsVersionsError.value = e?.message ?? String(e);
    } finally {
      skillsVersionsDeleting.value.delete(id);
      skillsVersionsDeleting.value = new Map(skillsVersionsDeleting.value);
    }
  }

  return {
    skillsVersionsLoading,
    skillsVersionsError,
    skillsVersionsData,
    skillsVersionsList,

    skillsVersionNewId,
    skillsVersionNewNote,
    skillsVersionsCreating,
    skillsVersionsDeleting,

    refreshSkillVersions,
    createSkillVersionFromForm,
    deleteSkillVersionByID,
  };
}


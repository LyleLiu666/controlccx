import { computed, ref } from "vue";
import type {
  InstallGitBatchItem,
  OnboardingPlan,
  SkillsToolInfo,
} from "../types";
import {
  fetchSkillsOnboarding,
  fetchSkillsTools,
  importExistingSkill,
  installSkillGitBatch,
  installSkillLocal,
  listGitSkillCandidates,
  syncSkill,
  updateManagedSkill,
} from "../api";

type SkillTarget = "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";

export function useSkillsGovernance() {
  const toolsLoading = ref(false);
  const toolsError = ref("");
  const tools = ref<SkillsToolInfo[]>([]);

  const onboardingLoading = ref(false);
  const onboardingError = ref("");
  const onboarding = ref<OnboardingPlan | null>(null);

  const actionError = ref("");
  const actionInfo = ref("");

  // Import existing
  const importName = ref("");
  const importTool = ref("");
  const importSourcePath = ref("");
  const importOverwrite = ref(false);
  const importing = ref(false);

  // Local install
  const localSourcePath = ref("");
  const localName = ref("");
  const localOverwrite = ref(false);
  const installingLocal = ref(false);

  // Git install
  const gitRepoURL = ref("");
  const gitOverwrite = ref(false);
  const gitTargets = ref<SkillTarget[]>([]);
  const gitCandidatesLoading = ref(false);
  const gitCandidatesError = ref("");
  const gitCandidates = ref<
    Array<{
      subpath: string;
      description?: string;
      selected: boolean;
      name: string;
      default_name: string;
    }>
  >([]);
  const installingGit = ref(false);

  // Sync
  const syncName = ref("");
  const syncTarget = ref<SkillTarget>("claude_code");
  const syncOverwrite = ref(false);
  const syncing = ref(false);

  // Update
  const updateName = ref("");
  const updating = ref(false);

  const hasOnboarding = computed(() => (onboarding.value?.groups?.length ?? 0) > 0);

  function resetActionMessages() {
    actionError.value = "";
    actionInfo.value = "";
  }

  async function refreshTools() {
    toolsError.value = "";
    toolsLoading.value = true;
    try {
      const res = await fetchSkillsTools();
      tools.value = res.tools ?? [];
    } catch (e: any) {
      toolsError.value = e?.message ?? String(e);
    } finally {
      toolsLoading.value = false;
    }
  }

  async function refreshOnboarding() {
    onboardingError.value = "";
    onboardingLoading.value = true;
    try {
      onboarding.value = await fetchSkillsOnboarding();
    } catch (e: any) {
      onboardingError.value = e?.message ?? String(e);
    } finally {
      onboardingLoading.value = false;
    }
  }

  async function runImportExisting() {
    if (importing.value) return;
    importing.value = true;
    resetActionMessages();
    try {
      const res = await importExistingSkill({
        name: importName.value.trim(),
        tool: importTool.value.trim(),
        source_path: importSourcePath.value.trim(),
        overwrite: importOverwrite.value,
      });
      actionInfo.value = `已接管技能：${res.name}`;
      await refreshOnboarding();
    } catch (e: any) {
      actionError.value = e?.message ?? String(e);
    } finally {
      importing.value = false;
    }
  }

  async function runInstallLocal() {
    if (installingLocal.value) return;
    installingLocal.value = true;
    resetActionMessages();
    try {
      const res = await installSkillLocal({
        source_path: localSourcePath.value.trim(),
        name: localName.value.trim() || undefined,
        overwrite: localOverwrite.value,
      });
      actionInfo.value = `本地安装完成：${res.name}`;
    } catch (e: any) {
      actionError.value = e?.message ?? String(e);
    } finally {
      installingLocal.value = false;
    }
  }

  async function runListGitCandidates() {
    if (gitCandidatesLoading.value) return;
    gitCandidatesError.value = "";
    gitCandidatesLoading.value = true;
    try {
      const res = await listGitSkillCandidates({ repo_url: gitRepoURL.value.trim() });
      const cands = res.candidates ?? [];
      const autoSelect = cands.length === 1;
      gitCandidates.value = cands.map((c) => ({
        subpath: c.subpath,
        description: c.description,
        selected: autoSelect,
        name: c.name,
        default_name: c.name,
      }));
    } catch (e: any) {
      gitCandidatesError.value = e?.message ?? String(e);
      gitCandidates.value = [];
    } finally {
      gitCandidatesLoading.value = false;
    }
  }

  async function runInstallGitBatch() {
    if (installingGit.value) return;
    installingGit.value = true;
    resetActionMessages();
    try {
      const selected = gitCandidates.value.filter((c) => c.selected);
      if (selected.length === 0) {
        actionError.value = "请先选择至少一个候选技能"
        return;
      }
      const skills: InstallGitBatchItem[] = selected.map((c) => ({
        subpath: c.subpath,
        name: c.name.trim() || undefined,
      }));
      const res = await installSkillGitBatch({
        repo_url: gitRepoURL.value.trim(),
        skills,
        targets: gitTargets.value,
        overwrite: gitOverwrite.value,
      });
      const names = (res.installed ?? []).map((s) => s?.name).filter(Boolean);
      actionInfo.value = names.length ? `Git 安装完成：${names.join(", ")}` : "Git 安装完成";
    } catch (e: any) {
      actionError.value = e?.message ?? String(e);
    } finally {
      installingGit.value = false;
    }
  }

  async function runSync() {
    if (syncing.value) return;
    syncing.value = true;
    resetActionMessages();
    try {
      await syncSkill({
        name: syncName.value.trim(),
        target: syncTarget.value,
        overwrite: syncOverwrite.value,
      });
      actionInfo.value = `已同步：${syncName.value.trim()} → ${syncTarget.value}`;
    } catch (e: any) {
      actionError.value = e?.message ?? String(e);
    } finally {
      syncing.value = false;
    }
  }

  async function runUpdate() {
    if (updating.value) return;
    updating.value = true;
    resetActionMessages();
    try {
      const res = await updateManagedSkill({ name: updateName.value.trim() });
      actionInfo.value = `已更新：${res.name}`;
    } catch (e: any) {
      actionError.value = e?.message ?? String(e);
    } finally {
      updating.value = false;
    }
  }

  return {
    toolsLoading,
    toolsError,
    tools,
    onboardingLoading,
    onboardingError,
    onboarding,
    hasOnboarding,

    actionError,
    actionInfo,

    importName,
    importTool,
    importSourcePath,
    importOverwrite,
    importing,

    localSourcePath,
    localName,
    localOverwrite,
    installingLocal,

    gitRepoURL,
    gitOverwrite,
    gitTargets,
    gitCandidatesLoading,
    gitCandidatesError,
    gitCandidates,
    installingGit,

    syncName,
    syncTarget,
    syncOverwrite,
    syncing,

    updateName,
    updating,

    refreshTools,
    refreshOnboarding,
    runImportExisting,
    runInstallLocal,
    runListGitCandidates,
    runInstallGitBatch,
    runSync,
    runUpdate,
  };
}

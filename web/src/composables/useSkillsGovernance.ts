import { computed, ref } from "vue";
import type {
  GitSkillCandidate,
  OnboardingPlan,
  SkillsToolInfo,
} from "../types";
import {
  fetchSkillsOnboarding,
  fetchSkillsTools,
  importExistingSkill,
  installSkillGit,
  installSkillLocal,
  listGitSkillCandidates,
  syncSkill,
  updateManagedSkill,
} from "../api";

type SkillTarget = "cursor" | "claude_code" | "codex";

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
  const gitSubpath = ref("");
  const gitName = ref("");
  const gitOverwrite = ref(false);
  const gitCandidatesLoading = ref(false);
  const gitCandidatesError = ref("");
  const gitCandidates = ref<GitSkillCandidate[]>([]);
  const installingGit = ref(false);

  // Sync
  const syncName = ref("");
  const syncTarget = ref<SkillTarget>("cursor");
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
      actionInfo.value = `Imported: ${res.name}`;
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
      actionInfo.value = `Installed (local): ${res.name}`;
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
      gitCandidates.value = res.candidates ?? [];
      if (gitCandidates.value.length === 1) {
        gitSubpath.value = gitCandidates.value[0].subpath;
        if (!gitName.value.trim()) {
          gitName.value = gitCandidates.value[0].name;
        }
      }
    } catch (e: any) {
      gitCandidatesError.value = e?.message ?? String(e);
      gitCandidates.value = [];
    } finally {
      gitCandidatesLoading.value = false;
    }
  }

  async function runInstallGit() {
    if (installingGit.value) return;
    installingGit.value = true;
    resetActionMessages();
    try {
      const res = await installSkillGit({
        repo_url: gitRepoURL.value.trim(),
        subpath: gitSubpath.value.trim() || undefined,
        name: gitName.value.trim() || undefined,
        overwrite: gitOverwrite.value,
      });
      actionInfo.value = `Installed (git): ${res.name}`;
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
      actionInfo.value = `Synced: ${syncName.value.trim()} -> ${syncTarget.value}`;
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
      actionInfo.value = `Updated: ${res.name}`;
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
    gitSubpath,
    gitName,
    gitOverwrite,
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
    runInstallGit,
    runSync,
    runUpdate,
  };
}


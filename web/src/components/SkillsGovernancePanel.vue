<script setup lang="ts">
import { onMounted } from "vue";
import { useSkillsGovernance } from "../composables/useSkillsGovernance";

const gov = useSkillsGovernance();

onMounted(() => {
  void gov.refreshTools();
});
</script>

<template>
  <div class="skillsGovernanceCard">
    <div class="skillsGovernanceHeader">
      <div class="skillsGovernanceTitle">Governance</div>
      <div class="skillsGovernanceHeaderBtns">
        <button type="button" class="headerMiniBtn" @click="gov.refreshTools" :disabled="gov.toolsLoading">
          Tools
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="gov.refreshOnboarding"
          :disabled="gov.onboardingLoading"
        >
          Scan
        </button>
      </div>
    </div>

    <div v-if="gov.toolsError" class="modalError">{{ gov.toolsError }}</div>
    <div v-else-if="gov.toolsLoading" class="loading">Loading tools...</div>
    <template v-else>
      <div class="skillsGovernanceTools">
        <div v-for="t in gov.tools" :key="t.key" class="skillsGovToolRow">
          <div class="mono">{{ t.key }}</div>
          <span class="pill mono skillStatus" :class="t.installed ? 'ok' : 'dim'">
            {{ t.installed ? "INSTALLED" : "NOT INSTALLED" }}
          </span>
          <div class="tinyHint mono skillsGovToolRoots" :title="(t.skills_roots ?? []).join('\n')">
            {{ (t.skills_roots ?? []).join(" · ") }}
          </div>
        </div>
      </div>
    </template>

    <div v-if="gov.actionError" class="modalError">{{ gov.actionError }}</div>
    <div v-else-if="gov.actionInfo" class="tinyHint">{{ gov.actionInfo }}</div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Onboarding</div>
      <div v-if="gov.onboardingError" class="modalError">{{ gov.onboardingError }}</div>
      <div v-else-if="gov.onboardingLoading" class="loading">Scanning...</div>
      <template v-else>
        <div class="tinyHint" v-if="gov.onboarding">
          Tools scanned:
          <span class="mono">{{ gov.onboarding.total_tools_scanned }}</span>
          · Skills found:
          <span class="mono">{{ gov.onboarding.total_skills_found }}</span>
          · Groups:
          <span class="mono">{{ gov.onboarding.groups.length }}</span>
        </div>
        <div v-if="gov.hasOnboarding" class="skillsGovOnboardingList">
          <div v-for="g in gov.onboarding?.groups ?? []" :key="g.name" class="skillsGovOnboardingRow">
            <div class="mono">{{ g.name }}</div>
            <span class="pill mono skillStatus" :class="g.has_conflict ? 'warn' : 'ok'">
              {{ g.has_conflict ? "CONFLICT" : "OK" }}
            </span>
            <div class="tinyHint mono" :title="g.variants.map((v) => `${v.tool}:${v.path}`).join('\n')">
              {{ g.variants.length }} variant(s)
            </div>
          </div>
        </div>
        <div v-else class="tinyHint">No discovered skills (or all are already managed).</div>
      </template>
    </div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Import Existing</div>
      <div class="skillsGovForm">
        <input v-model="gov.importName" placeholder="name (required)" />
        <input v-model="gov.importTool" placeholder="tool key (optional) e.g. cursor" />
        <input v-model="gov.importSourcePath" placeholder="source path (required)" />
        <label class="skillsGovCheckbox">
          <input type="checkbox" v-model="gov.importOverwrite" />
          overwrite
        </label>
        <button type="button" class="primary" @click="gov.runImportExisting" :disabled="gov.importing">
          {{ gov.importing ? "Importing..." : "Import" }}
        </button>
      </div>
    </div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Install (Local)</div>
      <div class="skillsGovForm">
        <input v-model="gov.localSourcePath" placeholder="source path" />
        <input v-model="gov.localName" placeholder="name (optional)" />
        <label class="skillsGovCheckbox">
          <input type="checkbox" v-model="gov.localOverwrite" />
          overwrite
        </label>
        <button type="button" class="primary" @click="gov.runInstallLocal" :disabled="gov.installingLocal">
          {{ gov.installingLocal ? "Installing..." : "Install" }}
        </button>
      </div>
    </div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Install (Git)</div>
      <div class="skillsGovForm">
        <input v-model="gov.gitRepoURL" placeholder="repo url (git or GitHub)" />
        <button type="button" @click="gov.runListGitCandidates" :disabled="gov.gitCandidatesLoading">
          {{ gov.gitCandidatesLoading ? "..." : "List" }}
        </button>
      </div>
      <div v-if="gov.gitCandidatesError" class="modalError">{{ gov.gitCandidatesError }}</div>
      <div v-else-if="gov.gitCandidates.length" class="skillsGovCandidates">
        <label class="tinyHint">Candidates</label>
        <select v-model="gov.gitSubpath">
          <option value="">(use repo url path)</option>
          <option v-for="c in gov.gitCandidates" :key="c.subpath" :value="c.subpath">
            {{ c.name }} · {{ c.subpath }}
          </option>
        </select>
      </div>
      <div class="skillsGovForm">
        <input v-model="gov.gitName" placeholder="name (optional)" />
        <label class="skillsGovCheckbox">
          <input type="checkbox" v-model="gov.gitOverwrite" />
          overwrite
        </label>
        <button type="button" class="primary" @click="gov.runInstallGit" :disabled="gov.installingGit">
          {{ gov.installingGit ? "Installing..." : "Install" }}
        </button>
      </div>
      <div class="tinyHint">
        If you see <span class="mono">MULTI_SKILLS|...</span>, click “List” and install a candidate subpath.
      </div>
    </div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Sync (Take Over)</div>
      <div class="skillsGovForm">
        <input v-model="gov.syncName" placeholder="skill name" />
        <select v-model="gov.syncTarget">
          <option value="cursor">cursor</option>
          <option value="claude_code">claude_code</option>
          <option value="codex">codex</option>
        </select>
        <label class="skillsGovCheckbox">
          <input type="checkbox" v-model="gov.syncOverwrite" />
          overwrite
        </label>
        <button type="button" class="primary" @click="gov.runSync" :disabled="gov.syncing">
          {{ gov.syncing ? "Syncing..." : "Sync" }}
        </button>
      </div>
    </div>

    <div class="skillsGovSection">
      <div class="skillsGovSectionTitle">Update (From Source)</div>
      <div class="skillsGovForm">
        <input v-model="gov.updateName" placeholder="skill name" />
        <button type="button" class="primary" @click="gov.runUpdate" :disabled="gov.updating">
          {{ gov.updating ? "Updating..." : "Update" }}
        </button>
      </div>
    </div>
  </div>
</template>

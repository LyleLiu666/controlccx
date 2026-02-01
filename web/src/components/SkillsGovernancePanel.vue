<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useSkillsGovernance } from "../composables/useSkillsGovernance";

const gov = reactive(useSkillsGovernance());
const op = ref<"local" | "git" | "import" | "sync" | "update">("local");

const canImportExisting = computed(
  () => !!gov.importName.trim() && !!gov.importSourcePath.trim(),
);
const canInstallLocal = computed(() => !!gov.localSourcePath.trim());
const canInstallGit = computed(() => !!gov.gitRepoURL.trim());
const canSync = computed(() => !!gov.syncName.trim());
const canUpdate = computed(() => !!gov.updateName.trim());

onMounted(() => {
  void gov.refreshTools();
});
</script>

<template>
  <div class="skillsGovernanceCard">
    <div class="skillsGovernanceHeader">
      <div class="skillsGovernanceTitle">技能管理</div>
      <div class="skillsGovernanceHeaderBtns">
        <button type="button" class="headerMiniBtn" @click="gov.refreshTools" :disabled="gov.toolsLoading">
          环境
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="gov.refreshOnboarding"
          :disabled="gov.onboardingLoading"
        >
          扫描
        </button>
      </div>
    </div>

    <div class="tinyHint skillsGovIntro">
      用于统一管理 Cursor / Claude Code / Codex 的 skills。常见流程：先导入/安装技能 → 右侧列表启用到目标工具。
    </div>

    <div v-if="gov.actionError" class="modalError">操作失败：{{ gov.actionError }}</div>
    <div v-else-if="gov.actionInfo" class="tinyHint">{{ gov.actionInfo }}</div>

    <details class="skillsGovDetails">
      <summary class="skillsGovSummary">环境检查</summary>
      <div v-if="gov.toolsError" class="modalError">{{ gov.toolsError }}</div>
      <div v-else-if="gov.toolsLoading" class="loading">加载中…</div>
      <template v-else>
        <div class="skillsGovernanceTools">
          <div v-for="t in gov.tools" :key="t.key" class="skillsGovToolRow">
            <div class="mono">{{ t.key }}</div>
            <span class="pill mono skillStatus" :class="t.installed ? 'ok' : 'dim'">
              {{ t.installed ? "已安装" : "未安装" }}
            </span>
            <div class="tinyHint mono skillsGovToolRoots" :title="(t.skills_roots ?? []).join('\n')">
              {{ (t.skills_roots ?? []).join(" · ") }}
            </div>
          </div>
        </div>
      </template>
      <div class="tinyHint">
        注：这里展示的是各工具扫描到的 skills 根目录。未安装时，右侧可能无法启用对应目标。
      </div>
    </details>

    <details class="skillsGovDetails">
      <summary class="skillsGovSummary">扫描结果（可接管/冲突提示）</summary>
      <div v-if="gov.onboardingError" class="modalError">{{ gov.onboardingError }}</div>
      <div v-else-if="gov.onboardingLoading" class="loading">扫描中…</div>
      <template v-else>
        <div class="tinyHint" v-if="gov.onboarding">
          扫描工具：<span class="mono">{{ gov.onboarding.total_tools_scanned }}</span>
          · 发现技能：<span class="mono">{{ gov.onboarding.total_skills_found }}</span>
          · 组：<span class="mono">{{ gov.onboarding.groups.length }}</span>
        </div>
        <div v-if="gov.hasOnboarding" class="skillsGovOnboardingList">
          <div
            v-for="g in gov.onboarding?.groups ?? []"
            :key="g.name"
            class="skillsGovOnboardingRow"
          >
            <div class="mono">{{ g.name }}</div>
            <span class="pill mono skillStatus" :class="g.has_conflict ? 'warn' : 'ok'">
              {{ g.has_conflict ? "冲突" : "正常" }}
            </span>
            <div class="tinyHint mono" :title="g.variants.map((v) => `${v.tool}:${v.path}`).join('\n')">
              {{ g.variants.length }} 个变体
            </div>
          </div>
        </div>
        <div v-else class="tinyHint">暂无可接管技能（或已全部纳管）。</div>
      </template>
    </details>

    <details class="skillsGovDetails" open>
      <summary class="skillsGovSummary">导入 / 安装 / 同步</summary>

      <div class="skillsGovTabs">
        <button type="button" class="skillsGovTab" :class="{ active: op === 'local' }" @click="op = 'local'">
          本地安装
        </button>
        <button type="button" class="skillsGovTab" :class="{ active: op === 'git' }" @click="op = 'git'">
          Git 安装
        </button>
        <button type="button" class="skillsGovTab" :class="{ active: op === 'import' }" @click="op = 'import'">
          接管已有技能
        </button>
        <button type="button" class="skillsGovTab" :class="{ active: op === 'sync' }" @click="op = 'sync'">
          同步到目标
        </button>
        <button type="button" class="skillsGovTab" :class="{ active: op === 'update' }" @click="op = 'update'">
          从源更新
        </button>
      </div>

      <div v-if="op === 'local'" class="skillsGovOp">
        <div class="skillsGovSectionTitle">本地安装</div>
        <div class="tinyHint">
          从本机路径安装一个技能目录（例如包含 <span class="mono">SKILL.md</span> 的文件夹）。
        </div>
        <div class="skillsGovFields">
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              本地路径 <span class="skillsGovReq">*</span>
            </div>
            <input
              v-model="gov.localSourcePath"
              placeholder="例如：/path/to/skills/foo"
            />
          </div>
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">技能名（可选）</div>
            <input v-model="gov.localName" placeholder="默认从目录名推断" />
          </div>
          <div class="skillsGovActions">
            <label class="skillsGovCheckbox">
              <input type="checkbox" v-model="gov.localOverwrite" />
              覆盖同名
            </label>
            <button
              type="button"
              class="primary skillsGovPrimaryBtn"
              @click="gov.runInstallLocal"
              :disabled="gov.installingLocal || !canInstallLocal"
              :title="canInstallLocal ? '安装' : '请先填写本地路径'"
            >
              {{ gov.installingLocal ? "安装中…" : "安装" }}
            </button>
          </div>
        </div>
      </div>

      <div v-else-if="op === 'git'" class="skillsGovOp">
        <div class="skillsGovSectionTitle">Git 安装</div>
        <div class="tinyHint">从 Git 仓库安装技能（支持 GitHub URL）。</div>
        <div class="skillsGovFields">
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              仓库地址 <span class="skillsGovReq">*</span>
            </div>
            <div class="skillsGovRow">
              <input v-model="gov.gitRepoURL" placeholder="例如：https://github.com/user/repo" />
              <button
                type="button"
                class="skillsGovSecondaryBtn"
                @click="gov.runListGitCandidates"
                :disabled="gov.gitCandidatesLoading || !canInstallGit"
                :title="canInstallGit ? '列出候选子路径（如需要）' : '请先填写仓库地址'"
              >
                {{ gov.gitCandidatesLoading ? "…" : "列出候选" }}
              </button>
            </div>
          </div>
          <div v-if="gov.gitCandidatesError" class="modalError">{{ gov.gitCandidatesError }}</div>
          <div v-else-if="gov.gitCandidates.length" class="skillsGovCandidates">
            <label class="tinyHint">候选子路径</label>
            <select v-model="gov.gitSubpath">
              <option value="">（使用仓库 URL 的默认路径）</option>
              <option v-for="c in gov.gitCandidates" :key="c.subpath" :value="c.subpath">
                {{ c.name }} · {{ c.subpath }}
              </option>
            </select>
          </div>
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">技能名（可选）</div>
            <input v-model="gov.gitName" placeholder="留空则自动推断" />
          </div>
          <div class="skillsGovActions">
            <label class="skillsGovCheckbox">
              <input type="checkbox" v-model="gov.gitOverwrite" />
              覆盖同名
            </label>
            <button
              type="button"
              class="primary skillsGovPrimaryBtn"
              @click="gov.runInstallGit"
              :disabled="gov.installingGit || !canInstallGit"
              :title="canInstallGit ? '安装' : '请先填写仓库地址'"
            >
              {{ gov.installingGit ? "安装中…" : "安装" }}
            </button>
          </div>
          <div class="tinyHint">
            若提示 <span class="mono">MULTI_SKILLS|...</span>，请先点“列出候选”，再选择子路径安装。
          </div>
        </div>
      </div>

      <div v-else-if="op === 'import'" class="skillsGovOp">
        <div class="skillsGovSectionTitle">接管已有技能</div>
        <div class="tinyHint">把某个现有目录纳入管理（不会改动你的原始目录结构）。</div>
        <div class="skillsGovFields">
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              技能名 <span class="skillsGovReq">*</span>
            </div>
            <input v-model="gov.importName" placeholder="例如：my-skill" />
          </div>
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">工具 key（可选）</div>
            <input v-model="gov.importTool" placeholder="例如：cursor / claude_code / codex" />
          </div>
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              源路径 <span class="skillsGovReq">*</span>
            </div>
            <input v-model="gov.importSourcePath" placeholder="例如：/path/to/skill" />
          </div>
          <div class="skillsGovActions">
            <label class="skillsGovCheckbox">
              <input type="checkbox" v-model="gov.importOverwrite" />
              覆盖同名
            </label>
            <button
              type="button"
              class="primary skillsGovPrimaryBtn"
              @click="gov.runImportExisting"
              :disabled="gov.importing || !canImportExisting"
              :title="canImportExisting ? '接管' : '请先填写技能名和源路径'"
            >
              {{ gov.importing ? "接管中…" : "接管" }}
            </button>
          </div>
        </div>
      </div>

      <div v-else-if="op === 'sync'" class="skillsGovOp">
        <div class="skillsGovSectionTitle">同步到目标</div>
        <div class="tinyHint">把已纳管的技能同步到目标工具（用于接管/覆盖某个目标目录）。</div>
        <div class="skillsGovFields">
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              技能名 <span class="skillsGovReq">*</span>
            </div>
            <input v-model="gov.syncName" placeholder="例如：code-review-excellence" />
          </div>
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">目标工具</div>
            <select v-model="gov.syncTarget">
              <option value="cursor">Cursor</option>
              <option value="claude_code">Claude Code</option>
              <option value="codex">Codex</option>
            </select>
          </div>
          <div class="skillsGovActions">
            <label class="skillsGovCheckbox">
              <input type="checkbox" v-model="gov.syncOverwrite" />
              覆盖同名
            </label>
            <button
              type="button"
              class="primary skillsGovPrimaryBtn"
              @click="gov.runSync"
              :disabled="gov.syncing || !canSync"
              :title="canSync ? '同步' : '请先填写技能名'"
            >
              {{ gov.syncing ? "同步中…" : "同步" }}
            </button>
          </div>
        </div>
      </div>

      <div v-else-if="op === 'update'" class="skillsGovOp">
        <div class="skillsGovSectionTitle">从源更新</div>
        <div class="tinyHint">从来源更新一个已纳管技能（例如 Git 仓库拉取更新）。</div>
        <div class="skillsGovFields">
          <div class="skillsGovField">
            <div class="skillsGovFieldLabel">
              技能名 <span class="skillsGovReq">*</span>
            </div>
            <input v-model="gov.updateName" placeholder="例如：code-review-excellence" />
          </div>
          <div class="skillsGovActions">
            <button
              type="button"
              class="primary skillsGovPrimaryBtn"
              @click="gov.runUpdate"
              :disabled="gov.updating || !canUpdate"
              :title="canUpdate ? '更新' : '请先填写技能名'"
            >
              {{ gov.updating ? "更新中…" : "更新" }}
            </button>
          </div>
        </div>
      </div>
    </details>
  </div>
</template>

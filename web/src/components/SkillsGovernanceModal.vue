<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useSkillsGovernance } from "../composables/useSkillsGovernance";
import { detectSkillInstallKind, normalizeGitRepoURL } from "../skillInstall";

const props = defineProps<{ open: boolean; prefill?: { name?: string } | null }>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const gov = reactive(useSkillsGovernance());
const op = ref<"local" | "git">("local");
const advancedOp = ref<"import" | "sync" | "update">("import");

type NoticeKind = "success" | "error" | "info";

const noticeOpen = ref(false);
const noticeTitle = ref("提示");
const noticeMessage = ref("");
const noticeLines = ref<string[]>([]);
const noticeKind = ref<NoticeKind>("info");

const canImportExisting = computed(
  () => !!gov.importName.trim() && !!gov.importSourcePath.trim(),
);
const canInstallLocal = computed(() => !!gov.localSourcePath.trim());
const canInstallGit = computed(() => !!gov.gitRepoURL.trim());
const canSync = computed(() => !!gov.syncName.trim());
const canUpdate = computed(() => !!gov.updateName.trim());
const gitSelectedCount = computed(() => (gov.gitCandidates ?? []).filter((c) => c.selected).length);
const gitAllSelected = computed(
  () => (gov.gitCandidates?.length ?? 0) > 0 && gitSelectedCount.value === gov.gitCandidates.length,
);
const gitTargetTools = computed(() =>
  (gov.tools ?? []).filter(
    (t) =>
      t.key === "claude_code" ||
      t.key === "codex" ||
      t.key === "antigravity" ||
      t.key === "opencode" ||
      t.key === "cursor",
  ),
);

let switchingInstallOp = false;
watch(
  () => gov.localSourcePath,
  (value) => {
    if (switchingInstallOp) return;
    if (op.value !== "local") return;
    if (detectSkillInstallKind(value) !== "git") return;
    switchingInstallOp = true;
    gov.gitRepoURL = value;
    gov.localSourcePath = "";
    op.value = "git";
    switchingInstallOp = false;
  },
);
watch(
  () => gov.gitRepoURL,
  (value) => {
    if (switchingInstallOp) return;
    if (op.value !== "git") return;
    if (detectSkillInstallKind(value) !== "local") return;
    switchingInstallOp = true;
    gov.localSourcePath = value;
    gov.gitRepoURL = "";
    op.value = "local";
    switchingInstallOp = false;
  },
);

async function listGitCandidates() {
  gov.gitRepoURL = normalizeGitRepoURL(gov.gitRepoURL);
  await gov.runListGitCandidates();
}

function openNotice(opts: { title?: string; message: string; lines?: string[]; kind?: NoticeKind }) {
  noticeTitle.value = String(opts.title ?? "").trim() || "提示";
  noticeMessage.value = String(opts.message ?? "").trim();
  noticeLines.value = opts.lines ?? [];
  noticeKind.value = opts.kind ?? "info";
  noticeOpen.value = true;
}

function closeNotice() {
  noticeOpen.value = false;
  noticeMessage.value = "";
  noticeLines.value = [];
}

function parseKnownError(raw: string): { prefix: string; payload: string } | null {
  const s = String(raw ?? "");
  for (const prefix of ["TARGET_EXISTS|", "MULTI_SKILLS|", "TOOL_NOT_INSTALLED|"]) {
    const idx = s.indexOf(prefix);
    if (idx === -1) continue;
    return { prefix, payload: s.slice(idx + prefix.length).trim() };
  }
  return null;
}

function basename(p: string): string {
  const s = String(p ?? "").trim().replace(/[/\\]+$/, "");
  if (!s) return "";
  const parts = s.split(/[/\\]/).filter(Boolean);
  return parts[parts.length - 1] ?? s;
}

function formatSkillsGovError(raw: string): { message: string; lines: string[] } {
  const msg = String(raw ?? "").trim();
  const hit = parseKnownError(msg);
  if (!hit) return { message: msg || "操作失败", lines: [] };

  switch (hit.prefix) {
    case "TARGET_EXISTS|": {
      const path = hit.payload;
      const name = basename(path);
      return {
        message: "检测到同名技能已存在，且未勾选“覆盖同名”，本次操作未执行。",
        lines: [name ? `同名：${name}` : "", path ? `路径：${path}` : "", "提示：你可以勾选“覆盖同名”或修改技能名。"].filter(
          Boolean,
        ),
      };
    }
    case "TOOL_NOT_INSTALLED|": {
      const tool = hit.payload || "unknown";
      return {
        message: `未检测到目标工具已安装：${tool}`,
        lines: ["请先安装/配置该工具，或选择其他目标。"],
      };
    }
    case "MULTI_SKILLS|": {
      return {
        message: "该仓库包含多个 Skills：请先点“列出候选”，再勾选要安装的技能。",
        lines: hit.payload ? [hit.payload] : [],
      };
    }
    default:
      return { message: msg || "操作失败", lines: [] };
  }
}

async function installLocal() {
  const res = await gov.runInstallLocal();
  if (gov.actionError) {
    const info = formatSkillsGovError(gov.actionError);
    openNotice({ title: "安装失败", message: info.message, lines: info.lines, kind: "error" });
    return;
  }
  openNotice({
    title: "安装成功",
    message: "安装成功",
    lines: res?.name ? [`技能：${res.name}`] : [],
    kind: "success",
  });
}

function setGitSelectAll(checked: boolean) {
  for (const c of gov.gitCandidates ?? []) {
    c.selected = checked;
  }
}

async function installGitSelected() {
  gov.gitRepoURL = normalizeGitRepoURL(gov.gitRepoURL);
  const selected = (gov.gitCandidates ?? []).filter((c) => c.selected);
  const requestedNames = selected.map((c) => String(c.name ?? "").trim() || String(c.default_name ?? "").trim());

  const res = await gov.runInstallGitBatch();
  if (gov.actionError) {
    const info = formatSkillsGovError(gov.actionError);
    openNotice({ title: "安装失败", message: info.message, lines: info.lines, kind: "error" });
    return;
  }

  const installed = res?.installed ?? [];
  const installedNames = installed.map((s) => String(s?.name ?? "").trim()).filter(Boolean);
  const renamed: string[] = [];
  for (let i = 0; i < requestedNames.length && i < installed.length; i++) {
    const before = requestedNames[i];
    const after = String(installed[i]?.name ?? "").trim();
    if (!before || !after || before === after) continue;
    renamed.push(`${before} → ${after}`);
  }

  const lines: string[] = [];
  if (installedNames.length) lines.push(`技能：${installedNames.join(", ")}`);
  if (renamed.length) {
    lines.push("同名冲突已自动改名：");
    lines.push(...renamed);
    lines.push("如需覆盖，请勾选“覆盖同名”。");
  }

  openNotice({ title: "安装成功", message: "安装成功", lines, kind: "success" });
}

async function importExisting() {
  const res = await gov.runImportExisting();
  if (gov.actionError) {
    const info = formatSkillsGovError(gov.actionError);
    openNotice({ title: "操作失败", message: info.message, lines: info.lines, kind: "error" });
    return;
  }
  openNotice({
    title: "操作成功",
    message: "接管成功",
    lines: res?.name ? [`技能：${res.name}`] : [],
    kind: "success",
  });
}

async function sync() {
  await gov.runSync();
  if (gov.actionError) {
    const info = formatSkillsGovError(gov.actionError);
    openNotice({ title: "操作失败", message: info.message, lines: info.lines, kind: "error" });
    return;
  }
  openNotice({ title: "操作成功", message: "同步成功", lines: gov.actionInfo ? [gov.actionInfo] : [], kind: "success" });
}

async function updateFromSource() {
  const res = await gov.runUpdate();
  if (gov.actionError) {
    const info = formatSkillsGovError(gov.actionError);
    openNotice({ title: "操作失败", message: info.message, lines: info.lines, kind: "error" });
    return;
  }
  openNotice({
    title: "操作成功",
    message: "更新成功",
    lines: res?.name ? [`技能：${res.name}`] : [],
    kind: "success",
  });
}

const didInit = ref(false);
watch(
  () => props.open,
  (open) => {
    if (!open) return;
    if (didInit.value) return;
    didInit.value = true;
    void gov.refreshTools();
  },
  { immediate: true },
);

watch(
  () => props.open,
  (open) => {
    if (open) return;
    closeNotice();
  },
);

watch(
  () => [props.open, props.prefill?.name],
  ([open, name]) => {
    if (!open) return;
    const n = String(name ?? "").trim();
    if (!n) return;
    gov.localName = n;
    gov.importName = n;
    gov.syncName = n;
    gov.updateName = n;
  },
);
</script>

<template>
  <div v-show="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal skillsGovModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">技能管理</div>
        <button
          type="button"
          class="headerMiniBtn"
          @click="gov.refreshTools"
          :disabled="gov.toolsLoading"
          title="刷新环境检查"
        >
          环境
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="gov.refreshOnboarding"
          :disabled="gov.onboardingLoading"
          title="扫描可接管技能"
        >
          扫描
        </button>
        <button class="iconBtn" type="button" @click="emit('close')" aria-label="关闭">
          ✕
        </button>
      </div>

      <div class="modalBody skillsGovModalBody">
        <div class="tinyHint skillsGovIntro">
          用于统一管理 Cursor / Claude Code / Codex 的 skills。常见流程：先导入/安装技能 → 右侧列表启用到目标工具。
        </div>

        <div v-if="gov.actionError" class="modalError">操作失败：{{ gov.actionError }}</div>
        <div v-else-if="gov.actionInfo" class="tinyHint">{{ gov.actionInfo }}</div>

        <div class="skillsGovPrimary">
          <div class="skillsGovPrimaryHeader">
            <div class="skillsGovPrimaryTitle">添加技能（常用）</div>
            <div class="tinyHint">从本地目录或 Git 仓库导入到来源库，再在列表里启用到目标工具。</div>
          </div>

          <div class="skillsGovTabs">
            <button
              type="button"
              class="skillsGovTab"
              :class="{ active: op === 'local' }"
              @click="op = 'local'"
            >
              本地安装
            </button>
            <button
              type="button"
              class="skillsGovTab"
              :class="{ active: op === 'git' }"
              @click="op = 'git'"
            >
              Git 安装
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
                <input v-model="gov.localSourcePath" placeholder="例如：/path/to/skills/foo" />
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
                  @click="installLocal"
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
            <div class="tinyHint">从 Git 仓库安装技能（支持 GitHub URL，支持输入 owner/repo）。</div>
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
                    @click="listGitCandidates"
                    :disabled="gov.gitCandidatesLoading || !canInstallGit"
                    :title="canInstallGit ? '列出候选子路径（如需要）' : '请先填写仓库地址'"
                  >
                    {{ gov.gitCandidatesLoading ? "…" : "列出候选" }}
                  </button>
                </div>
              </div>
              <div v-if="gov.gitCandidatesError" class="modalError">{{ gov.gitCandidatesError }}</div>
              <div v-else-if="gov.gitCandidates.length" class="skillsGovCandidates">
                <div class="skillsGovCandidatesHeader">
                  <label class="tinyHint">
                    候选技能（已选 {{ gitSelectedCount }} / {{ gov.gitCandidates.length }}）
                  </label>
                  <label class="skillsGovCheckbox">
                    <input
                      type="checkbox"
                      :checked="gitAllSelected"
                      @change="setGitSelectAll(($event.target as HTMLInputElement).checked)"
                    />
                    全选
                  </label>
                </div>
                <div class="skillsGovCandidateList">
                  <div v-for="c in gov.gitCandidates" :key="c.subpath" class="skillsGovCandidateRow">
                    <input type="checkbox" v-model="c.selected" />
                    <div class="skillsGovCandidateBody">
                      <input
                        v-model="c.name"
                        class="skillsGovCandidateName"
                        :placeholder="c.default_name || '技能名'"
                      />
                      <div class="tinyHint mono">{{ c.subpath }}</div>
                      <div v-if="c.description" class="tinyHint">{{ c.description }}</div>
                    </div>
                  </div>
                </div>

                <div class="skillsGovField">
                  <div class="skillsGovFieldLabel">同步到目标（可选）</div>
                  <div class="skillsGovTargets">
                    <label v-for="t in gitTargetTools" :key="t.key" class="skillsGovCheckbox">
                      <input
                        type="checkbox"
                        :value="t.key"
                        v-model="gov.gitTargets"
                        :disabled="!t.installed"
                      />
                      {{ t.display_name }}<span v-if="!t.installed" class="tinyHint">（未检测到）</span>
                    </label>
                  </div>
                </div>
              </div>
              <div class="skillsGovActions">
                <label class="skillsGovCheckbox">
                  <input type="checkbox" v-model="gov.gitOverwrite" />
                  覆盖同名
                </label>
                <button
                  type="button"
                  class="primary skillsGovPrimaryBtn"
                  @click="installGitSelected"
                  :disabled="gov.installingGit || !canInstallGit || gitSelectedCount === 0"
                  :title="canInstallGit ? '批量安装所选' : '请先填写仓库地址'"
                >
                  {{ gov.installingGit ? "安装中…" : "安装所选" }}
                </button>
              </div>
              <div class="tinyHint">
                若提示 <span class="mono">MULTI_SKILLS|...</span>，请先点“列出候选”，再勾选要安装的技能。
              </div>
            </div>
          </div>
        </div>

        <details class="skillsGovDetails">
          <summary class="skillsGovSummary">高级操作（接管 / 同步 / 从源更新）</summary>
          <div class="skillsGovTabs">
            <button
              type="button"
              class="skillsGovTab"
              :class="{ active: advancedOp === 'import' }"
              @click="advancedOp = 'import'"
            >
              接管已有技能
            </button>
            <button
              type="button"
              class="skillsGovTab"
              :class="{ active: advancedOp === 'sync' }"
              @click="advancedOp = 'sync'"
            >
              同步到目标
            </button>
            <button
              type="button"
              class="skillsGovTab"
              :class="{ active: advancedOp === 'update' }"
              @click="advancedOp = 'update'"
            >
              从源更新
            </button>
          </div>

          <div v-if="advancedOp === 'import'" class="skillsGovOp">
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
                  @click="importExisting"
                  :disabled="gov.importing || !canImportExisting"
                  :title="canImportExisting ? '接管' : '请先填写技能名和源路径'"
                >
                  {{ gov.importing ? "接管中…" : "接管" }}
                </button>
              </div>
            </div>
          </div>

          <div v-else-if="advancedOp === 'sync'" class="skillsGovOp">
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
                  <option value="claude_code">Claude Code</option>
                  <option value="codex">Codex</option>
                  <option value="antigravity">Antigravity</option>
                  <option value="opencode">OpenCode</option>
                  <option value="cursor">Cursor</option>
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
                  @click="sync"
                  :disabled="gov.syncing || !canSync"
                  :title="canSync ? '同步' : '请先填写技能名'"
                >
                  {{ gov.syncing ? "同步中…" : "同步" }}
                </button>
              </div>
            </div>
          </div>

          <div v-else class="skillsGovOp">
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
                  @click="updateFromSource"
                  :disabled="gov.updating || !canUpdate"
                  :title="canUpdate ? '更新' : '请先填写技能名'"
                >
                  {{ gov.updating ? "更新中…" : "更新" }}
                </button>
              </div>
            </div>
          </div>
        </details>

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
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">关闭</button>
      </div>
    </div>

    <div v-if="noticeOpen" class="modalOverlay" @click.self="closeNotice">
      <div class="modal smallModal" role="dialog" aria-modal="true">
        <div class="modalHeader">
          <div class="modalTitle">{{ noticeTitle }}</div>
          <button class="iconBtn" type="button" @click="closeNotice" aria-label="关闭">✕</button>
        </div>
        <div class="modalBody">
          <div v-if="noticeKind === 'error'" class="modalError">{{ noticeMessage }}</div>
          <div v-else class="confirmText">{{ noticeMessage }}</div>
          <div v-if="noticeLines.length" class="tinyHint mono">
            <div v-for="(line, idx) in noticeLines" :key="idx">{{ line }}</div>
          </div>
        </div>
        <div class="modalFooter">
          <button type="button" class="primary" @click="closeNotice">确定</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { fetchSkills } from "../api";
import { type SkillTarget, type SkillsSummary, summarizeSkillTarget } from "../skillsSummary";
import { formatSkillToken } from "../skillsInvoke";
import type { Skill, Tool, ToolDriver } from "../types";
import {
  isHighRiskPreset,
  safetyPresetsForDriver,
  toolDriverForWorkerType,
} from "../runSafety";

const props = defineProps<{
  open: boolean;
  workdir: string;
  prompt: string;
  missingAuthText: string;
  toolsList: Tool[];
  toolsError: string;
  toolsLoading: boolean;
  workerType: string;
  safetyOverride: boolean;
  installUnlock: boolean;
  autopilotEnabled: boolean;
  safetyPreset: string;
  highRiskOptIn: boolean;
  starting: boolean;
  highRiskConfirmOpen: boolean;
}>();

const emit = defineEmits<{
  (e: "update:open", val: boolean): void;
  (e: "update:workdir", val: string): void;
  (e: "update:prompt", val: string): void;
  (e: "update:workerType", val: string): void;
  (e: "update:safetyOverride", val: boolean): void;
  (e: "update:installUnlock", val: boolean): void;
  (e: "update:autopilotEnabled", val: boolean): void;
  (e: "update:safetyPreset", val: string): void;
  (e: "update:highRiskOptIn", val: boolean): void;
  (e: "close"): void;
  (e: "create"): void;
  (e: "openDirPicker"): void;
  (e: "openAuthSettings"): void;
}>();

const newRunDriver = computed<ToolDriver>(() =>
  toolDriverForWorkerType(props.workerType, props.toolsList)
);

const newTool = computed(() => {
  return props.toolsList.find((t) => t.id === props.workerType) ?? null;
});

const newRunUseAutopilot = computed<boolean>(
  () => props.autopilotEnabled && !props.safetyOverride
);

const newRunShowManualSafety = computed<boolean>(
  () => !newRunUseAutopilot.value
);

function close() {
  closeSkillsPicker();
  emit("update:open", false);
  emit("close");
}

function updateWorkdir(e: Event) {
  emit("update:workdir", (e.target as HTMLInputElement).value);
}

function updatePrompt(e: Event) {
  emit("update:prompt", (e.target as HTMLTextAreaElement).value);
}

const canUseSkills = computed<boolean>(() => newRunDriver.value === "claude-code" || newRunDriver.value === "codex");
const skillsPickerOpen = ref(false);
const skillsQuery = ref("");
const skillsOnlyAvailable = ref(true);
const skillsLoading = ref(false);
const skillsError = ref("");
const skillsData = ref<Skill[]>([]);
const skillsSearchEl = ref<HTMLInputElement | null>(null);

const skillsTarget = computed<SkillTarget | null>(() => {
  switch (newRunDriver.value) {
    case "claude-code":
      return "claude_code";
    case "codex":
      return "codex";
    default:
      return null;
  }
});

function skillStatusLabel(status: SkillsSummary["status"]): string {
  switch (status) {
    case "missing":
      return "缺失";
    case "linked":
      return "已关联";
    case "copied":
      return "已复制";
    case "present":
      return "已存在";
    case "external":
      return "外部";
    case "broken":
      return "异常";
    case "conflict":
      return "冲突";
    case "partial":
      return "部分";
    default:
      return String(status).toUpperCase();
  }
}

function skillStatusBadgeClass(status: SkillsSummary["status"]): string {
  switch (status) {
    case "linked":
    case "copied":
      return "ok";
    case "present":
    case "external":
      return "muted";
    case "partial":
      return "partial";
    case "missing":
      return "dim";
    default:
      return "warn";
  }
}

function isSkillAvailable(summary: SkillsSummary): boolean {
  return summary.status !== "missing" && summary.status !== "broken";
}

const skillsVisible = computed(() => {
  const q = skillsQuery.value.trim().toLowerCase();
  const target = skillsTarget.value;

  const out: Array<{ name: string; summary: SkillsSummary | null; available: boolean; title: string }> = [];
  for (const s of skillsData.value) {
    if (q && !String(s.name ?? "").toLowerCase().includes(q)) continue;

    const summary = target ? summarizeSkillTarget(s, target) : null;
    const available = summary ? isSkillAvailable(summary) : false;
    if (skillsOnlyAvailable.value && !available) continue;
    out.push({
      name: s.name,
      summary,
      available,
      title: summary?.detail ?? "",
    });
  }

  out.sort((a, b) => a.name.localeCompare(b.name));
  return out.slice(0, 200);
});

async function ensureSkillsLoaded(force = false) {
  if (!canUseSkills.value) return;
  if (skillsLoading.value) return;
  if (!force && skillsData.value.length) return;
  skillsLoading.value = true;
  skillsError.value = "";
  try {
    const res = await fetchSkills({ limit: 500, offset: 0 });
    const list = Array.isArray(res.skills) ? res.skills.slice() : [];
    list.sort((a, b) => String(a.name).localeCompare(String(b.name)));
    skillsData.value = list;
  } catch (e: any) {
    skillsError.value = e?.message ?? String(e);
  } finally {
    skillsLoading.value = false;
  }
}

function openSkillsPicker() {
  if (!canUseSkills.value) return;
  skillsPickerOpen.value = true;
  skillsError.value = "";
  void ensureSkillsLoaded();
  void nextTick(() => {
    skillsSearchEl.value?.focus();
  });
}

function closeSkillsPicker() {
  skillsPickerOpen.value = false;
  skillsError.value = "";
  skillsQuery.value = "";
}

function onSkillsPickerKeyDown(ev: KeyboardEvent) {
  if (!skillsPickerOpen.value) return;
  if (ev.key !== "Escape") return;
  ev.preventDefault();
  closeSkillsPicker();
  void nextTick(() => {
    promptEl.value?.focus();
  });
}

function insertIntoPrompt(text: string) {
  const current = String(props.prompt ?? "");
  const el = promptEl.value;
  const start = el?.selectionStart ?? current.length;
  const end = el?.selectionEnd ?? start;
  const next = current.slice(0, start) + text + current.slice(end);
  emit("update:prompt", next);
  void nextTick(() => {
    const nextPos = start + text.length;
    try {
      el?.focus();
      el?.setSelectionRange(nextPos, nextPos);
    } catch {
      // ignore
    }
  });
}

function chooseSkill(name: string) {
  if (!canUseSkills.value) return;
  const token = formatSkillToken(name, newRunDriver.value);
  if (!token) return;

  const insertText = token.startsWith("/") ? token + "\n" : token + " ";
  insertIntoPrompt(insertText);
  closeSkillsPicker();
}

function atBlankLineStart(el: HTMLTextAreaElement, value: string): boolean {
  const pos = el.selectionStart ?? 0;
  const before = value.slice(0, pos);
  const lineStart = before.lastIndexOf("\n") + 1;
  const prefix = before.slice(lineStart);
  return /^\s*$/.test(prefix);
}

function onPromptKeyDown(ev: KeyboardEvent) {
  if (ev.key !== "/") return;
  if (ev.ctrlKey || ev.metaKey || ev.altKey) return;
  if (!canUseSkills.value) return;
  const el = ev.target as HTMLTextAreaElement | null;
  if (!el) return;

  const current = String(props.prompt ?? "");
  if (!atBlankLineStart(el, current)) return;
  ev.preventDefault();
  openSkillsPicker();
}

// Focus handling for prompt textarea
const promptEl = ref<HTMLTextAreaElement | null>(null);
watch(
  () => props.open,
  async (open) => {
    if (open) {
      // Small delay to allow render
      setTimeout(() => {
        if (skillsPickerOpen.value) return;
        promptEl.value?.focus();
      }, 50);
      return;
    }
    closeSkillsPicker();
  }
);
</script>

<template>
  <div
    v-if="open"
    class="modalOverlay newRunOverlay"
    @click.self="close"
  >
    <div class="modal newRunModal">
      <div class="modalHeader">
        <div class="modalTitle">新建运行</div>
        <button class="iconBtn" type="button" @click="close">✕</button>
      </div>

      <div class="modalBody newRunBody">
        <div class="form newRunForm">
          <label class="full">
            工作目录
            <div class="workdirRow">
              <input
                :value="workdir"
                @input="updateWorkdir"
                placeholder="."
              />
              <button type="button" @click="emit('openDirPicker')">浏览</button>
            </div>
          </label>
          <div v-if="missingAuthText" class="authHint full">
            <div class="text">{{ missingAuthText }}</div>
            <button type="button" @click="emit('openAuthSettings')">
              认证设置
            </button>
          </div>
          <label class="full">
            <div class="newRunPromptLabelRow">
              <span>提示词</span>
              <button
                type="button"
                class="inlineBtn newRunSkillsOpenBtn"
                @click="openSkillsPicker"
                :disabled="!canUseSkills"
                :title="canUseSkills ? '选择技能（将插入到提示词中）' : '当前工具不支持 skills'"
              >
                选择技能
              </button>
            </div>
            <textarea
              ref="promptEl"
              class="promptEmphasis"
              :value="prompt"
              @input="updatePrompt"
              @keydown="onPromptKeyDown"
              rows="6"
              placeholder="描述要运行的任务..."
            ></textarea>
            <div v-if="canUseSkills" class="tinyHint newRunSkillsHint">
              技能：点击「选择技能」或在输入框里按 <span class="mono">/</span> 搜索（兼容 TUI）。
            </div>
            <div v-else class="tinyHint newRunSkillsHint">
              当前工具为 <span class="mono">{{ newRunDriver }}</span>：不支持 skills（仅执行命令）。
            </div>
          </label>

          <div
            v-if="skillsPickerOpen"
            class="modalOverlay newRunSkillsOverlay"
            @click.self="closeSkillsPicker"
            @keydown.capture="onSkillsPickerKeyDown"
          >
            <div class="modal smallModal newRunSkillsModal" role="dialog" aria-modal="true">
              <div class="modalHeader">
                <div class="modalTitle">选择技能</div>
                <button class="iconBtn" type="button" @click="closeSkillsPicker">✕</button>
              </div>
              <div class="modalBody newRunSkillsBody">
                <div class="tinyHint">
                  当前工具：<span class="mono">{{ newRunDriver }}</span> · 插入：
                  <span class="mono">{{ formatSkillToken("my-skill", newRunDriver) }}</span>
                </div>

                <div class="newRunSkillsToolbar">
                  <input
                    ref="skillsSearchEl"
                    v-model="skillsQuery"
                    placeholder="搜索技能（名称）…"
                    :disabled="skillsLoading"
                  />
                  <label class="newRunSkillsOnly">
                    <input type="checkbox" v-model="skillsOnlyAvailable" />
                    <span>仅显示可用</span>
                  </label>
                  <button
                    type="button"
                    @click="ensureSkillsLoaded(true)"
                    :disabled="skillsLoading"
                    title="刷新技能列表"
                  >
                    刷新
                  </button>
                </div>

                <div v-if="skillsError" class="modalError">{{ skillsError }}</div>
                <div v-else-if="skillsLoading" class="loading">加载中…</div>
                <div v-else class="newRunSkillsList" role="listbox" aria-label="Skills">
                  <button
                    v-for="s in skillsVisible"
                    :key="s.name"
                    type="button"
                    class="newRunSkillOption"
                    :disabled="!s.available"
                    :title="s.title"
                    @click="chooseSkill(s.name)"
                  >
                    <span class="mono">{{ s.name }}</span>
                    <span v-if="s.summary" class="pill mono skillStatus" :class="skillStatusBadgeClass(s.summary.status)">{{
                      skillStatusLabel(s.summary.status)
                    }}</span>
                    <span v-else class="pill mono skillStatus dim">未知</span>
                  </button>
                  <div v-if="!skillsVisible.length" class="empty">暂无匹配技能</div>
                </div>
              </div>
              <div class="modalFooter">
                <button type="button" @click="closeSkillsPicker">关闭</button>
              </div>
            </div>
          </div>
          <details class="newRunAdvanced full">
            <summary>
              <span>高级设置</span>
              <span class="newRunAdvancedSummaryHint">
                <template v-if="toolsError">工具错误</template>
                <template v-else-if="toolsLoading">加载工具中…</template>
                <template v-else
                  >工具: <span class="mono">{{ workerType }}</span></template
                >
              </span>
            </summary>
            <div class="newRunAdvancedBody">
              <label>
                工具
                <select
                  :value="workerType"
                  @change="emit('update:workerType', ($event.target as HTMLSelectElement).value)"
                >
                  <option v-for="t in toolsList" :key="t.id" :value="t.id">
                    {{ t.id }}
                  </option>
                </select>
              </label>
              <div v-if="toolsError" class="tinyHint warn">{{ toolsError }}</div>
              <div v-else-if="toolsLoading" class="tinyHint">加载工具中…</div>
              <div v-else-if="newTool" class="tinyHint">
                驱动: <span class="mono">{{ newTool.driver }}</span> · 命令:
                <span class="mono">{{ newTool.command }}</span>
              </div>

              <div
                v-if="newRunDriver === 'codex' || newRunDriver === 'claude-code'"
                class="newRunSafety"
              >
                <div class="newRunSafetyTitle">安全设置</div>
                <div class="newRunSafetyAutoRow">
                  <span
                    class="pill"
                    :class="autopilotEnabled ? 'low' : 'warn'"
                    >自动驾驶</span
                  >
                  <span class="tinyHint">
                    <template v-if="autopilotEnabled">
                      根据提示词推断意图并应用最佳实践沙箱默认值。
                    </template>
                    <template v-else>
                      自动驾驶已禁用：请在下方选择安全预设。
                    </template>
                  </span>
                </div>

                <label class="newRunSafetyUnlock">
                  <input
                    type="checkbox"
                    :checked="installUnlock"
                    @change="emit('update:installUnlock', ($event.target as HTMLInputElement).checked)"
                  />
                  <span class="mono">安装解锁 (Install unlock)</span>
                  <span class="tinyHint">开启下载/安装权限（允许 agent 下载/安装依赖）</span>
                </label>

                <div class="newRunSafetyAdvanced">
                  <div class="newRunSafetyAdvancedGrid">
                    <label class="full">
                      <input
                        type="checkbox"
                        :checked="autopilotEnabled"
                        @change="emit('update:autopilotEnabled', ($event.target as HTMLInputElement).checked)"
                      />
                      <span>安全自动驾驶（推荐）</span>
                    </label>
                    <label v-if="autopilotEnabled" class="full">
                      <input
                        type="checkbox"
                        :checked="safetyOverride"
                        @change="emit('update:safetyOverride', ($event.target as HTMLInputElement).checked)"
                      />
                      <span>覆盖自动驾驶（手动设置预设）</span>
                    </label>
                  </div>

                  <template v-if="newRunShowManualSafety">
                    <div class="newRunSafetyGrid">
                      <label>
                        安全预设
                        <select
                          :value="safetyPreset"
                          @change="emit('update:safetyPreset', ($event.target as HTMLSelectElement).value)"
                        >
                          <option
                            v-for="p in safetyPresetsForDriver(newRunDriver)"
                            :key="p.value"
                            :value="p.value"
                          >
                            {{ p.label }}
                          </option>
                        </select>
                      </label>
                    </div>

                    <div
                      v-if="
                        newRunDriver === 'claude-code' &&
                        safetyPreset === 'search-browse'
                      "
                      class="tinyHint"
                    >
                      启用 Claude Code WebFetch。默认禁止通过
                      <span class="mono">curl</span>/<span class="mono"
                        >wget</span
                      >
                      下载。
                    </div>
                    <div
                      v-else-if="
                        newRunDriver === 'codex' &&
                        safetyPreset === 'search-browse'
                      "
                      class="tinyHint"
                    >
                      启用 Codex <span class="mono">--search</span> (原生
                      web_search 工具)。搜索/浏览不同于下载/执行脚本。
                    </div>

                    <div
                      v-if="isHighRiskPreset(newRunDriver, safetyPreset)"
                      class="newRunSafetyWarn"
                    >
                      <div class="tinyHint warn">
                        <template
                          v-if="
                            newRunDriver === 'codex' && safetyPreset === 'unsafe'
                          "
                        >
                          将以 <span class="mono">--dangerously-bypass-approvals-and-sandbox</span> 运行：跳过审批并关闭 sandbox 隔离，agent 可直接执行命令并访问系统资源（文件/网络）。
                        </template>
                        <template
                          v-else-if="
                            newRunDriver === 'codex' &&
                            safetyPreset === 'danger-full-access'
                          "
                        >
                          将以 <span class="mono">--sandbox danger-full-access</span> 运行：允许访问 workspace 外的文件/目录（权限更大）。
                        </template>
                        <template
                          v-else-if="
                            newRunDriver === 'claude-code' &&
                            safetyPreset === 'unsafe'
                          "
                        >
                          将以 <span class="mono">--dangerously-skip-permissions</span> 运行：跳过权限确认，并关闭 bash sandbox（脚本可直接访问系统文件/网络）。
                        </template>
                      </div>
                      <label class="newRunSafetyOptIn">
                        <input
                          type="checkbox"
                          :checked="highRiskOptIn"
                          @change="emit('update:highRiskOptIn', ($event.target as HTMLInputElement).checked)"
                        />
                        <span>我已知晓将开放的权限并希望继续</span>
                      </label>
                    </div>
                  </template>
                </div>
              </div>

              <div class="newRunHint">
                快捷键: <span class="mono">N</span> 新建运行 ·
                <span class="mono">S</span> 秘书 ·
                <span class="mono">L</span> 实时 ·
                <span class="mono">Esc</span> 关闭
              </div>
            </div>
          </details>
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="close">取消</button>
        <button
          type="button"
          class="primary"
          @click="emit('create')"
          :disabled="
            !prompt.trim() ||
            !workdir.trim() ||
            !!missingAuthText ||
            highRiskConfirmOpen ||
            starting
          "
        >
          {{ starting ? "启动中…" : "开始" }}
        </button>
      </div>
    </div>
  </div>
</template>

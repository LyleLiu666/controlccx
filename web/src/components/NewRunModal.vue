<script setup lang="ts">
import { computed, ref, watch } from "vue";
import SkillsInsertModal from "./SkillsInsertModal.vue";
import WorkdirCombobox from "./WorkdirCombobox.vue";
import type { PromptTemplate, Tool, ToolDriver } from "../types";
import { fetchPromptTemplates } from "../api";
import {
  isHighRiskPreset,
  safetyPresetsForDriver,
  toolDriverForWorkerType,
} from "../runSafety";

type WorkdirOption = { value: string; label: string; subLabel?: string };

const props = defineProps<{
  open: boolean;
  workdir: string;
  prompt: string;
  workdirPinnedOptions: WorkdirOption[];
  workdirRecentOptions: WorkdirOption[];
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

const taskTemplates = ref<PromptTemplate[]>([]);
const taskTemplatesLoading = ref(false);
const taskTemplatesError = ref("");
const selectedTaskTemplateID = ref("");

async function loadTaskTemplates() {
  if (taskTemplatesLoading.value) return;
  taskTemplatesLoading.value = true;
  taskTemplatesError.value = "";
  try {
    taskTemplates.value = await fetchPromptTemplates("task");
  } catch (e: any) {
    taskTemplatesError.value = e?.message ?? String(e);
  } finally {
    taskTemplatesLoading.value = false;
  }
}

function applyTaskTemplate() {
  const id = String(selectedTaskTemplateID.value ?? "").trim();
  if (!id) return;
  const tpl = taskTemplates.value.find((t) => t.id === id);
  if (!tpl) return;
  emit("update:prompt", String(tpl.content ?? ""));
}

function close() {
  closeSkillsPicker();
  emit("update:open", false);
  emit("close");
}

function updatePrompt(e: Event) {
  emit("update:prompt", (e.target as HTMLTextAreaElement).value);
}

const canUseSkills = computed<boolean>(() => newRunDriver.value === "claude-code" || newRunDriver.value === "codex");
const skillsPickerOpen = ref(false);

function openSkillsPicker() {
  if (!canUseSkills.value) return;
  skillsPickerOpen.value = true;
}

function closeSkillsPicker() {
  skillsPickerOpen.value = false;
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
      selectedTaskTemplateID.value = "";
      void loadTaskTemplates();
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
              <WorkdirCombobox
                :modelValue="workdir"
                :pinned="workdirPinnedOptions"
                :recent="workdirRecentOptions"
                placeholder="."
                @update:modelValue="emit('update:workdir', $event)"
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
            <div class="newRunTemplatesRow">
              <span class="tinyHint">模板</span>
              <select
                v-model="selectedTaskTemplateID"
                :disabled="taskTemplatesLoading || !taskTemplates.length"
                title="选择一个 task 模板并应用到提示词"
              >
                <option value="">(选择任务模板…)</option>
                <option v-for="t in taskTemplates" :key="t.id" :value="t.id">
                  {{ t.title }}
                </option>
              </select>
              <button
                type="button"
                @click="applyTaskTemplate"
                :disabled="taskTemplatesLoading || !selectedTaskTemplateID"
              >
                应用
              </button>
              <span v-if="taskTemplatesLoading" class="tinyHint">加载中…</span>
            </div>
            <div v-if="taskTemplatesError" class="tinyHint warn">{{ taskTemplatesError }}</div>
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
            <div class="tinyHint">
              Project Context（如已设置）会自动注入到运行 prompt（压缩/限长），不改写任务列表里的提示词。
            </div>
          </label>

          <SkillsInsertModal
            :open="skillsPickerOpen"
            :driver="newRunDriver"
            :prompt="prompt"
            :promptEl="promptEl"
            @close="closeSkillsPicker"
            @update:prompt="emit('update:prompt', $event)"
          />
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
                      启用 Claude Code WebFetch/WebSearch，并允许在 bash 中使用
                      <span class="mono">curl</span>/<span class="mono"
                        >wget</span
                      >
                      进行下载/请求（仍受 Claude bash sandbox 限制）。
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

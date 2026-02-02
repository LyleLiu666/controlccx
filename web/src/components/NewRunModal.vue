<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { Tool, ToolDriver, TaskIntent } from "../types";
import {
  isHighRiskPreset,
  recommendSafetyPreset,
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
  taskIntent: TaskIntent;
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
  (e: "update:taskIntent", val: TaskIntent): void;
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
  emit("update:open", false);
  emit("close");
}

function updateWorkdir(e: Event) {
  emit("update:workdir", (e.target as HTMLInputElement).value);
}

function updatePrompt(e: Event) {
  emit("update:prompt", (e.target as HTMLTextAreaElement).value);
}

// Focus handling for prompt textarea
const promptEl = ref<HTMLTextAreaElement | null>(null);
watch(
  () => props.open,
  async (open) => {
    if (open) {
      // Small delay to allow render
      setTimeout(() => {
        promptEl.value?.focus();
      }, 50);
    }
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
            提示词
            <textarea
              ref="promptEl"
              :value="prompt"
              @input="updatePrompt"
              rows="6"
              placeholder="描述要运行的任务..."
            ></textarea>
          </label>
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
                      自动驾驶已禁用：请在下方选择意图/预设。
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
                  <span class="tinyHint">允许下载/安装程序（较高风险）</span>
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
                      <span>覆盖自动驾驶（手动设置意图/预设）</span>
                    </label>
                  </div>

                  <template v-if="newRunShowManualSafety">
                    <div class="newRunSafetyGrid">
                      <label>
                        任务意图
                        <select
                          :value="taskIntent"
                          @change="emit('update:taskIntent', ($event.target as HTMLSelectElement).value as TaskIntent)"
                        >
                          <option value="code">code</option>
                          <option value="analyze">analyze</option>
                          <option value="search-browse">search-browse</option>
                          <option value="install">install</option>
                        </select>
                      </label>
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
                            {{ p.value }}
                          </option>
                        </select>
                      </label>
                    </div>
                    <div class="tinyHint">
                      推荐:
                      <span class="mono">{{
                        recommendSafetyPreset(newRunDriver, taskIntent)
                      }}</span>
                      <button
                        type="button"
                        class="inlineBtn"
                        @click="emit('update:safetyPreset', recommendSafetyPreset(newRunDriver, taskIntent))"
                      >
                        使用
                      </button>
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
                          以 <span class="mono">--dangerously-bypass-approvals-and-sandbox</span> (无沙箱) 运行 Codex。
                        </template>
                        <template
                          v-else-if="
                            newRunDriver === 'codex' &&
                            safetyPreset === 'danger-full-access'
                          "
                        >
                          以 <span class="mono">--sandbox danger-full-access</span> (可访问工作区外部) 运行 Codex。
                        </template>
                        <template
                          v-else-if="
                            newRunDriver === 'claude-code' &&
                            safetyPreset === 'unsafe'
                          "
                        >
                          以 <span class="mono">--dangerously-skip-permissions</span> 运行 Claude Code。建议仅在无互联网访问的沙箱中使用。
                        </template>
                      </div>
                      <label class="newRunSafetyOptIn">
                        <input
                          type="checkbox"
                          :checked="highRiskOptIn"
                          @change="emit('update:highRiskOptIn', ($event.target as HTMLInputElement).checked)"
                        />
                        <span>我已知晓风险并希望继续</span>
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

<script setup lang="ts">
import { computed } from "vue";
import type { Tool } from "../types";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  saving: boolean;
  error: string;
  tools: Tool[];
  selectedID: string;
  editCommand: string;
  editArgs: string;
  editEnv: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "refresh"): void;
  (e: "selectTool", tool: Tool): void;
  (e: "reset"): void;
  (e: "save"): void;
  (e: "update:editCommand", value: string): void;
  (e: "update:editArgs", value: string): void;
  (e: "update:editEnv", value: string): void;
}>();

const editCommandModel = computed({
  get: () => props.editCommand,
  set: (value: string) => emit("update:editCommand", value),
});
const editArgsModel = computed({
  get: () => props.editArgs,
  set: (value: string) => emit("update:editArgs", value),
});
const editEnvModel = computed({
  get: () => props.editEnv,
  set: (value: string) => emit("update:editEnv", value),
});

const selectedTool = computed<Tool | null>(() => {
  const id = String(props.selectedID ?? "").trim();
  if (!id) return props.tools?.[0] ?? null;
  return props.tools?.find((t) => t.id === id) ?? props.tools?.[0] ?? null;
});
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal toolsModal">
      <div class="modalHeader">
        <div class="modalTitle">工具设置</div>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('refresh')"
          :disabled="loading || saving"
        >
          刷新
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody toolsBody">
        <div class="setupHint">
          <div><strong>这里能做什么？</strong></div>
          <ol class="setupSteps">
            <li>只支持配置 Claude Code / Codex 的可执行文件路径（command）。</li>
            <li>保存后立即生效；若要回到默认值，点“恢复默认”。</li>
            <li>通常情况下你不需要改参数/环境变量；优先在“提供方/认证设置”里配置授权与模型。</li>
          </ol>
        </div>

        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading">加载中...</div>
        <template v-else>
          <div class="toolsSplit">
            <div class="toolsList">
              <div class="toolsListTitleRow">
                <div class="tinyHint">工具列表（点击切换）</div>
              </div>
                <button
                  v-for="t in tools"
                  :key="t.id"
                  type="button"
                  class="toolsItem"
                  :class="{ active: t.id === selectedID }"
                  @click="emit('selectTool', t)"
                  :title="t.command"
                >
                  <div class="mono">{{ t.id }}</div>
                <div class="tinyHint mono">{{ t.driver }}</div>
              </button>
            </div>

            <div class="toolsEditor">
              <div class="toolsEditorGrid">
                <label>
                  工具
                  <input :value="selectedTool?.id ?? ''" disabled />
                </label>
                <label class="full">
                  命令
                  <input
                    v-model="editCommandModel"
                    placeholder="例如：claude / codex / /opt/homebrew/bin/claude"
                    autocomplete="off"
                  />
                </label>
                <label class="full">
                  参数（空格分隔）
                  <textarea
                    v-model="editArgsModel"
                    rows="2"
                    placeholder="--foo --bar"
                  ></textarea>
                </label>
                <label class="full">
                  环境变量（每行一个 KEY=VALUE）
                  <textarea
                    v-model="editEnvModel"
                    rows="6"
                    placeholder="ANTHROPIC_BASE_URL=https://..."
                  ></textarea>
                </label>
                <div class="tinyHint">保存按钮需要填写：命令。</div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">关闭</button>
        <button type="button" @click="emit('reset')" :disabled="saving || !selectedID.trim()">
          恢复默认
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving || !selectedID.trim() || !editCommand.trim()"
        >
          {{ saving ? "保存中..." : "保存" }}
        </button>
      </div>
    </div>
  </div>
</template>

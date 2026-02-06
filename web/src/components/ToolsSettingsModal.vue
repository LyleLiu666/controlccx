<script setup lang="ts">
import { computed } from "vue";
import type { Tool, ToolDriver } from "../types";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  saving: boolean;
  error: string;
  tools: Tool[];
  editID: string;
  editDriver: ToolDriver;
  editCommand: string;
  editArgs: string;
  editEnv: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "newTool"): void;
  (e: "refresh"): void;
  (e: "selectTool", tool: Tool): void;
  (e: "delete"): void;
  (e: "save"): void;
  (e: "update:editID", value: string): void;
  (e: "update:editDriver", value: ToolDriver): void;
  (e: "update:editCommand", value: string): void;
  (e: "update:editArgs", value: string): void;
  (e: "update:editEnv", value: string): void;
}>();

const editIDModel = computed({
  get: () => props.editID,
  set: (value: string) => emit("update:editID", value),
});
const editDriverModel = computed({
  get: () => props.editDriver,
  set: (value: ToolDriver) => emit("update:editDriver", value),
});
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
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal toolsModal">
      <div class="modalHeader">
        <div class="modalTitle">工具设置</div>
        <button type="button" class="headerMiniBtn" @click="emit('newTool')">
          新建
        </button>
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
          <div><strong>怎么新增工具？</strong></div>
          <ol class="setupSteps">
            <li>先点“新建”，填写 `id`、`command`，driver 建议默认 `exec`。</li>
            <li>再点“保存”；保存成功后会出现在左侧工具列表。</li>
            <li>`claude-code` / `codex` 是系统内置入口，通常只需改命令或环境变量。</li>
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
                :class="{ active: t.id === editID }"
                @click="emit('selectTool', t)"
                :title="t.command"
              >
                <div class="mono">{{ t.id }}</div>
                <div class="tinyHint mono">{{ t.driver }}</div>
              </button>
            </div>

            <div class="toolsEditor">
              <div class="toolsEditorGrid">
                <label class="full">
                  工具 ID
                  <input
                    v-model="editIDModel"
                    placeholder="my-tool（唯一）"
                    autocomplete="off"
                  />
                </label>
                <label>
                  驱动
                  <select v-model="editDriverModel">
                    <option value="claude-code">claude-code</option>
                    <option value="codex">codex</option>
                    <option value="exec">exec</option>
                  </select>
                </label>
                <label class="full">
                  命令
                  <input
                    v-model="editCommandModel"
                    placeholder="例如：claude / codex / bash"
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
                <div class="tinyHint">保存按钮需要填写：工具 ID + 命令。</div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">关闭</button>
        <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">
          删除
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving || !editID.trim() || !editCommand.trim()"
        >
          {{ saving ? "保存中..." : "保存" }}
        </button>
      </div>
    </div>
  </div>
</template>

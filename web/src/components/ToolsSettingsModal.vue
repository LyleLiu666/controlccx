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
        <div class="modalTitle">Tools</div>
        <button type="button" class="headerMiniBtn" @click="emit('newTool')">
          New
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('refresh')"
          :disabled="loading || saving"
        >
          Refresh
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody toolsBody">
        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading">Loading...</div>
        <template v-else>
          <div class="toolsSplit">
            <div class="toolsList">
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
                  id
                  <input
                    v-model="editIDModel"
                    placeholder="my-tool"
                    autocomplete="off"
                  />
                </label>
                <label>
                  driver
                  <select v-model="editDriverModel">
                    <option value="claude-code">claude-code</option>
                    <option value="codex">codex</option>
                    <option value="exec">exec</option>
                  </select>
                </label>
                <label class="full">
                  command
                  <input
                    v-model="editCommandModel"
                    placeholder="claude"
                    autocomplete="off"
                  />
                </label>
                <label class="full">
                  args (space separated)
                  <textarea
                    v-model="editArgsModel"
                    rows="2"
                    placeholder="--foo --bar"
                  ></textarea>
                </label>
                <label class="full">
                  env (KEY=VALUE per line)
                  <textarea
                    v-model="editEnvModel"
                    rows="6"
                    placeholder="ANTHROPIC_BASE_URL=https://..."
                  ></textarea>
                </label>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">Close</button>
        <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">
          Delete
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving || !editID.trim() || !editCommand.trim()"
        >
          {{ saving ? "Saving..." : "Save" }}
        </button>
      </div>
    </div>
  </div>
</template>


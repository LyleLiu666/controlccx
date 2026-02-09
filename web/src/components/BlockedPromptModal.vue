<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
  warning: string;
  confirmOpen: boolean;
  safeRetryEnabled: boolean;
}>();

const WORKSPACE_REQUIRED_WARNING_PREFIX = "CCX_WORKSPACE_REQUIRED:";
const workspaceRequired = computed(() =>
  String(props.warning ?? "").trim().startsWith(WORKSPACE_REQUIRED_WARNING_PREFIX),
);
const warningText = computed(() => {
  const raw = String(props.warning ?? "").trim();
  if (!raw) return "";
  if (!workspaceRequired.value) return raw;
  return raw.slice(WORKSPACE_REQUIRED_WARNING_PREFIX.length).trim();
});

const emit = defineEmits<{
  (e: "close"): void;
  (e: "copyConfigSnippet"): void;
  (e: "proceed"): void;
  (e: "safeRetry"): void;
}>();
</script>

<template>
  <div
    v-if="props.open"
    class="modalOverlay"
    @click.self="emit('close')"
  >
    <div class="modal smallModal">
      <div class="modalHeader">
        <div class="modalTitle">
          {{ workspaceRequired ? "需要 workspace 才能写入" : "运行被阻塞（需要确认）" }}
        </div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>
      <div class="modalBody">
        <div v-if="props.error" class="modalError">
          {{ props.error }}
        </div>
        <template v-if="workspaceRequired">
          <div class="confirmText">
            需要 workspace 才能写入，避免直接修改 base workdir。
          </div>
          <div class="tinyHint">
            点击下方按钮后将创建 worktree/workspace，并继续该 run（写入将发生在隔离目录）。
          </div>
        </template>
        <template v-else>
          <div class="confirmText">
            这个 run 在执行过程中触发了需要人工确认的权限/操作，但当前运行是非交互模式，无法点击批准。
          </div>
          <div class="tinyHint">
            你可以先选择「保持当前安全设置重试」，用审批弹窗逐项批准；如果你确认需要开放更高权限，再选择「高权限继续」跳过权限确认（权限更大）。
          </div>
        </template>
        <div v-if="warningText" class="tinyHint warn mono">
          {{ warningText }}
        </div>
      </div>
      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">
          稍后
        </button>
        <template v-if="workspaceRequired">
          <button
            type="button"
            class="primary"
            @click="emit('proceed')"
            :disabled="props.busy || props.confirmOpen"
          >
            {{ props.busy ? "处理中..." : "创建 worktree/workspace 并继续" }}
          </button>
        </template>
        <template v-else>
          <button
            type="button"
            @click="emit('copyConfigSnippet')"
            :disabled="props.busy"
          >
            复制配置片段
          </button>
          <button
            v-if="props.safeRetryEnabled"
            type="button"
            class="primary"
            @click="emit('safeRetry')"
            :disabled="props.busy || props.confirmOpen"
          >
            {{ props.busy ? "处理中..." : "保持当前安全设置重试" }}
          </button>
          <button
            type="button"
            class="warnBtn"
            @click="emit('proceed')"
            :disabled="props.busy || props.confirmOpen"
          >
            {{ props.busy ? "处理中..." : "高权限继续" }}
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

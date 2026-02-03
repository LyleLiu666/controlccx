<script setup lang="ts">
const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
  warning: string;
  confirmOpen: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "copyConfigSnippet"): void;
  (e: "proceed"): void;
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
        <div class="modalTitle">运行被阻塞（需要确认）</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>
      <div class="modalBody">
        <div v-if="props.error" class="modalError">
          {{ props.error }}
        </div>
        <div class="confirmText">
          这个 run 在执行过程中触发了需要人工确认的权限/操作，但当前运行是非交互模式，无法点击批准。
        </div>
        <div class="tinyHint">
          如果你确认需要开放权限，可以选择「高权限继续」跳过权限确认（权限更大）。
        </div>
        <div v-if="props.warning" class="tinyHint warn mono">
          {{ props.warning }}
        </div>
      </div>
      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">
          稍后
        </button>
        <button
          type="button"
          @click="emit('copyConfigSnippet')"
          :disabled="props.busy"
        >
          复制配置片段
        </button>
        <button
          type="button"
          class="warnBtn"
          @click="emit('proceed')"
          :disabled="props.busy || props.confirmOpen"
        >
          {{ props.busy ? "处理中..." : "高权限继续" }}
        </button>
      </div>
    </div>
  </div>
</template>


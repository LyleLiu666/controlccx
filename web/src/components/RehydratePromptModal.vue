<script setup lang="ts">
const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "confirm"): void;
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
        <div class="modalTitle">无法恢复该会话</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>
      <div class="modalBody">
        <div v-if="props.error" class="modalError">
          {{ props.error }}
        </div>
        <div class="confirmText">
          Claude Code 找不到该 session（No conversation found）。你可以新建一个会话，把历史上下文带过去继续。
        </div>
        <div class="tinyHint">
          说明：该操作会创建一个新的 <span class="mono">mode=new</span> run（不会复用旧
          <span class="mono">session_id</span>）。
        </div>
      </div>
      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">
          取消
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('confirm')"
          :disabled="props.busy"
        >
          {{ props.busy ? "处理中..." : "新建会话继续（带上下文）" }}
        </button>
      </div>
    </div>
  </div>
</template>


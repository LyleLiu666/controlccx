<script setup lang="ts">
const props = defineProps<{
  open: boolean;
  title: string;
  message: string;
  detail: string;
  confirmLabel: string;
  busy: boolean;
}>();

const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "confirm"): void;
}>();
</script>

<template>
  <div
    v-if="props.open"
    class="modalOverlay"
    @click.self="emit('cancel')"
  >
    <div class="modal smallModal">
      <div class="modalHeader">
        <div class="modalTitle">{{ props.title }}</div>
        <button class="iconBtn" type="button" @click="emit('cancel')">✕</button>
      </div>
      <div class="modalBody">
        <div class="confirmText">{{ props.message }}</div>
        <div v-if="props.detail" class="tinyHint warn mono">
          {{ props.detail }}
        </div>
      </div>
      <div class="modalFooter">
        <button type="button" @click="emit('cancel')">取消</button>
        <button
          type="button"
          class="warnBtn"
          @click="emit('confirm')"
          :disabled="props.busy"
        >
          {{ props.confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>


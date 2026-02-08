<script setup lang="ts">
import { ref, watch } from "vue";

const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
  initialToken: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "save", token: string): void;
  (e: "clear"): void;
}>();

const token = ref("");

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    token.value = String(props.initialToken ?? "");
  },
);
</script>

<template>
  <div v-if="props.open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal smallModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">需要实例 Token</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody">
        <div v-if="props.error" class="modalError">{{ props.error }}</div>
        <div class="confirmText">
          服务器当前开启了远程访问保护：所有 <span class="mono">/api/*</span> 请求需要携带
          <span class="mono">X-ControlCCX-Token</span>。
        </div>
        <div class="tinyHint">
          Token 默认位于 <span class="mono">~/.controlccx/instance.token</span>。将其粘贴到下面即可继续使用 Web UI（含事件流 SSE）。
        </div>

        <label class="full">
          Token
          <input
            v-model="token"
            :disabled="props.busy"
            class="mono"
            placeholder="paste ~/.controlccx/instance.token"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">稍后</button>
        <button type="button" class="warnBtn" @click="emit('clear')" :disabled="props.busy">
          清除
        </button>
        <button type="button" class="primary" @click="emit('save', token)" :disabled="props.busy">
          {{ props.busy ? "验证中…" : "保存并继续" }}
        </button>
      </div>
    </div>
  </div>
</template>


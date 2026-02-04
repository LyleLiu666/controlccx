<script setup lang="ts">
type Largest = { path: string; bytes: number };

const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
  files: number;
  bytes: number;
  maxFiles: number;
  maxBytes: number;
  largest: Largest[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "skip"): void;
  (e: "force"): void;
}>();

function fmtBytes(v: number): string {
  const n = Number(v || 0);
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let x = n;
  while (x >= 1024 && i < units.length - 1) {
    x /= 1024;
    i++;
  }
  const digits = i === 0 ? 0 : x >= 10 ? 1 : 2;
  return `${x.toFixed(digits)} ${units[i]}`;
}
</script>

<template>
  <div
    v-if="props.open"
    class="modalOverlay"
    @click.self="emit('close')"
  >
    <div class="modal smallModal">
      <div class="modalHeader">
        <div class="modalTitle">Untracked 复制确认</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody">
        <div v-if="props.error" class="modalError">{{ props.error }}</div>

        <div class="confirmText">
          检测到当前仓库的 <span class="mono">untracked</span> 文件较多/较大，复制到 worktree 可能很慢。
        </div>

        <div class="tinyHint">
          当前：<span class="mono">{{ props.files }}</span> 个文件，<span class="mono">{{ fmtBytes(props.bytes) }}</span>
          <br />
          上限：<span class="mono">{{ props.maxFiles }}</span> 个文件，<span class="mono">{{ fmtBytes(props.maxBytes) }}</span>
        </div>

        <div v-if="props.largest?.length" class="tinyHint">
          最大的文件（Top {{ Math.min(5, props.largest.length) }}）：
          <div v-for="x in props.largest.slice(0, 5)" :key="x.path" class="tinyHint mono">
            {{ fmtBytes(x.bytes) }} · {{ x.path }}
          </div>
        </div>

        <div class="tinyHint">
          建议：如果这些文件是 <span class="mono">node_modules</span>、<span class="mono">.venv</span> 等可重建产物，可以选择跳过。
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">
          取消
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('skip')"
          :disabled="props.busy"
        >
          {{ props.busy ? "处理中..." : "继续但不复制 untracked" }}
        </button>
        <button
          type="button"
          @click="emit('force')"
          :disabled="props.busy"
        >
          仍然复制 untracked
        </button>
      </div>
    </div>
  </div>
</template>


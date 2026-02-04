<script setup lang="ts">
const props = defineProps<{
  open: boolean;
  busy: boolean;
  error: string;
  message: string;
  workdir: string;
  existingTaskID: string;
  existingStatus: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "wait"): void;
  (e: "worktree"): void;
  (e: "viewExisting"): void;
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
        <div class="modalTitle">工作目录被占用</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody">
        <div v-if="props.error" class="modalError">{{ props.error }}</div>

        <div class="confirmText">
          同一个 <span class="mono">workdir</span> 默认只能同时运行一个任务。
        </div>

        <div v-if="props.message" class="tinyHint warn mono">
          {{ props.message }}
        </div>

        <div class="tinyHint">
          Workdir: <span class="mono">{{ props.workdir }}</span>
        </div>
        <div class="tinyHint">
          占用的 run:
          <span class="mono">{{ props.existingTaskID.slice(0, 8) }}</span>
          <span class="pill" :class="props.existingStatus">{{ props.existingStatus }}</span>
        </div>

        <div class="tinyHint">
          你可以选择等待排队（更安全），或创建一个 Git worktree 并行开发（会复制未提交修改）。
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')" :disabled="props.busy">
          取消
        </button>
        <button type="button" @click="emit('viewExisting')" :disabled="props.busy">
          查看占用的 run
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('wait')"
          :disabled="props.busy"
        >
          {{ props.busy ? "处理中..." : "等待（排队）" }}
        </button>
        <button
          type="button"
          @click="emit('worktree')"
          :disabled="props.busy"
        >
          创建 Worktree 并行运行
        </button>
      </div>
    </div>
  </div>
</template>


<script setup lang="ts">
import type { RunningSessionCandidate } from "../runningSessions.ts";

const props = defineProps<{
  open: boolean;
  items: RunningSessionCandidate[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "select", runID: string): void;
}>();

function statusLabel(status: string): string {
  switch (status) {
    case "queued":
      return "排队中";
    case "waiting":
      return "等待中";
    case "running":
      return "运行中";
    default:
      return String(status);
  }
}
</script>

<template>
  <div v-if="props.open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal smallModal runningSessionsModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">运行中会话</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody">
        <div class="confirmText">
          检测到 <span class="mono">{{ props.items.length }}</span> 个会话仍在执行。点击一项可接续查看；
          点击空白处可直接新建会话。
        </div>

        <div class="runningSessionsList">
          <button
            v-for="it in props.items"
            :key="it.key"
            type="button"
            class="runningSessionsRow"
            @click="emit('select', it.run_id)"
          >
            <div class="runningSessionsRowTop">
              <span class="runningSessionsRowName" :title="it.title || it.workdir">{{
                it.title || it.workdir
              }}</span>
              <span class="pill mono" :class="it.status">{{ statusLabel(it.status) }}</span>
            </div>
            <div class="runningSessionsRowSub">
              <span class="mono runningSessionsRowWorkdir" :title="it.workdir">{{ it.workdir }}</span>
              <span v-if="it.in_flight_runs > 1" class="pill kind">{{ it.in_flight_runs }} 个运行</span>
            </div>
          </button>
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" class="primary" @click="emit('close')">关闭</button>
      </div>
    </div>
  </div>
</template>


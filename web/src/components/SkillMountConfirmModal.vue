<script setup lang="ts">
import type { ToolDriver } from "../types";

type SkillMountItem = {
  name: string;
  status: string;
  detail?: string;
};

const props = defineProps<{
  open: boolean;
  driver: ToolDriver;
  items: SkillMountItem[];
  busy: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "continue"): void;
  (e: "mount"): void;
}>();

function skillStatusLabel(status: string): string {
  switch (status) {
    case "missing":
      return "缺失";
    case "linked":
      return "已关联";
    case "copied":
      return "已复制";
    case "present":
      return "已存在";
    case "external":
      return "外部";
    case "broken":
      return "异常";
    case "conflict":
      return "冲突";
    case "partial":
      return "部分";
    default:
      return String(status).toUpperCase();
  }
}

function skillStatusBadgeClass(status: string): string {
  switch (status) {
    case "linked":
    case "copied":
      return "ok";
    case "present":
    case "external":
      return "muted";
    case "partial":
      return "partial";
    case "missing":
      return "dim";
    default:
      return "warn";
  }
}
</script>

<template>
  <div v-if="props.open" class="modalOverlay" @click.self="emit('cancel')">
    <div class="modal smallModal skillsMountModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">挂载技能？</div>
        <button class="iconBtn" type="button" @click="emit('cancel')">✕</button>
      </div>

      <div class="modalBody">
        <div class="confirmText">
          检测到以下技能在 <span class="mono">{{ props.driver }}</span> 下尚未挂载，是否现在挂载？
        </div>
        <div class="tinyHint">
          提示：执行时会自动兼容 <span class="mono">/skill</span> 与 <span class="mono">$skill</span>，不会改写你输入的
          prompt。
        </div>

        <div v-if="props.error" class="modalError">{{ props.error }}</div>

        <div class="skillsMountList">
          <div
            v-for="it in props.items"
            :key="it.name"
            class="skillsMountRow"
            :title="it.detail ?? ''"
          >
            <span class="mono skillsMountName">{{ it.name }}</span>
            <span class="pill mono skillStatus" :class="skillStatusBadgeClass(it.status)">{{
              skillStatusLabel(it.status)
            }}</span>
          </div>
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('cancel')" :disabled="props.busy">取消</button>
        <button type="button" @click="emit('continue')" :disabled="props.busy">继续（不挂载）</button>
        <button type="button" class="primary" @click="emit('mount')" :disabled="props.busy">
          {{ props.busy ? "挂载中…" : "挂载并继续" }}
        </button>
      </div>
    </div>
  </div>
</template>

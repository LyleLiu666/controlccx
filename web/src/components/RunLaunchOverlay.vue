<script setup lang="ts">
const props = defineProps<{
  open: boolean;
  title: string;
  detail?: string;
}>();
</script>

<template>
  <div
    v-if="props.open"
    class="runLaunchOverlay"
    role="status"
    aria-live="polite"
    aria-busy="true"
  >
    <div class="runLaunchCard">
      <div class="runLaunchSpinner" aria-hidden="true"></div>
      <div class="runLaunchTitle">{{ props.title }}</div>
      <div v-if="props.detail" class="runLaunchDetail">
        {{ props.detail }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.runLaunchOverlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-modal);
  backdrop-filter: blur(6px);
  display: grid;
  place-items: center;
  padding: max(24px, env(safe-area-inset-top)) 24px max(24px, env(safe-area-inset-bottom));
  z-index: 2000;
}

.runLaunchCard {
  width: min(520px, 92vw);
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 20px;
  box-shadow: var(--shadow-lg);
  padding: 20px 22px;
  display: grid;
  justify-items: center;
  gap: 10px;
  text-align: center;
  animation: runLaunchPopIn 0.2s ease-out;
}

.runLaunchSpinner {
  width: 42px;
  height: 42px;
  border-radius: 999px;
  border: 4px solid rgba(148, 163, 184, 0.35);
  border-top-color: var(--color-primary);
  animation: runLaunchSpin 0.9s linear infinite;
}

.runLaunchTitle {
  font-weight: 800;
  font-size: 16px;
  color: var(--text-main);
}

.runLaunchDetail {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-sub);
}

@keyframes runLaunchSpin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes runLaunchPopIn {
  from {
    opacity: 0;
    transform: scale(0.96);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .runLaunchSpinner {
    animation: none;
  }

  .runLaunchCard {
    animation: none;
  }
}
</style>


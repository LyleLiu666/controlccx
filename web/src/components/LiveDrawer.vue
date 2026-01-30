<script setup lang="ts">
import { computed, type Ref } from "vue";
import type { FeedItem } from "../composables/useLiveFeed";

const props = defineProps<{
  full: boolean;
  width: number;
  resizing: boolean;
  scope: "current" | "all";
  mode: "milestones" | "all";
  wrap: boolean;
  paused: boolean;
  items: FeedItem[];
  eventsConnected: boolean;
  eventsIdleSeconds: number;
  feedIdleSeconds: number;
  selectedTaskStatus: string;
  boxElRef: Ref<HTMLDivElement | null>;
  formatLogTime: (ts: string) => string;
  formatLocalDateTime: (ts: string) => string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "reconnect"): void;
  (e: "startResize", ev: MouseEvent): void;
  (e: "update:full", value: boolean): void;
  (e: "update:scope", value: "current" | "all"): void;
  (e: "update:mode", value: "milestones" | "all"): void;
  (e: "update:wrap", value: boolean): void;
  (e: "update:paused", value: boolean): void;
}>();

const fullModel = computed({
  get: () => props.full,
  set: (value: boolean) => emit("update:full", value),
});
const scopeModel = computed({
  get: () => props.scope,
  set: (value: "current" | "all") => emit("update:scope", value),
});
const modeModel = computed({
  get: () => props.mode,
  set: (value: "milestones" | "all") => emit("update:mode", value),
});
const wrapModel = computed({
  get: () => props.wrap,
  set: (value: boolean) => emit("update:wrap", value),
});
const pausedModel = computed({
  get: () => props.paused,
  set: (value: boolean) => emit("update:paused", value),
});
</script>

<template>
  <div class="secDrawerOverlay" @click.self="emit('close')">
    <aside
      class="secDrawer wide"
      :class="{ full: fullModel }"
      :style="{
        width: fullModel ? 'calc(100vw - 32px)' : width + 'px',
      }"
      role="dialog"
      aria-modal="true"
    >
      <div
        class="secResizeHandle"
        :class="{ active: resizing }"
        @mousedown="emit('startResize', $event)"
        title="Resize"
      ></div>
      <div class="secDrawerHeader">
        <div class="secDrawerTitle">Live</div>
        <button
          class="iconBtn"
          type="button"
          @click="fullModel = !fullModel"
          :title="fullModel ? 'Exit full screen' : 'Full screen'"
        >
          {{ fullModel ? "⤡" : "⤢" }}
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>
      <div class="secDrawerBody">
        <div class="secFeed">
          <div class="feedControls">
            <div class="feedLeft">
              <label class="feedLabel">
                Scope
                <select v-model="scopeModel">
                  <option value="current">Current</option>
                  <option value="all">All</option>
                </select>
              </label>
              <label class="feedLabel">
                View
                <select v-model="modeModel">
                  <option value="milestones">Milestones</option>
                  <option value="all">All Logs</option>
                </select>
              </label>
              <label class="feedToggle">
                <input type="checkbox" v-model="wrapModel" />
                Wrap
              </label>
              <button type="button" @click="pausedModel = !pausedModel">
                {{ pausedModel ? "Resume" : "Pause" }}
              </button>
            </div>
            <div class="feedRight">
              <span
                class="feedConn"
                :class="{ bad: !eventsConnected || eventsIdleSeconds >= 25 }"
                :title="
                  eventsConnected
                    ? `Connected · last event ${eventsIdleSeconds}s ago`
                    : `Disconnected · last event ${eventsIdleSeconds}s ago`
                "
              >
                {{ eventsConnected ? "Connected" : "Reconnecting…" }}
              </span>
              <button
                v-if="!eventsConnected || eventsIdleSeconds >= 25"
                type="button"
                class="feedReconnect"
                @click="emit('reconnect')"
                title="Reconnect event stream"
              >
                Reconnect
              </button>
              <span
                v-if="modeModel === 'milestones'"
                class="feedHint"
                title="Milestones show system run.start/run.finish, assistant output, and error-like lines."
              >
                Milestones
              </span>
              <span
                v-if="selectedTaskStatus === 'running' && feedIdleSeconds >= 10"
                class="feedIdle"
                :title="
                  feedIdleSeconds >= 300
                    ? `Quiet for ${feedIdleSeconds}s · tools may be silent`
                    : `No log output for ${feedIdleSeconds}s`
                "
              >
                {{ feedIdleSeconds >= 300 ? "Quiet" : "No logs" }}
                {{ feedIdleSeconds }}s
              </span>
            </div>
          </div>

          <div
            :ref="boxElRef"
            class="feedBox"
            :class="{ wrap: wrapModel }"
            role="log"
            aria-label="Live feed"
          >
            <div v-if="items.length === 0" class="empty">
              暂无日志（仅展示本次打开页面后收到的实时日志）
            </div>
            <div v-else class="feedLines">
              <div
                v-for="(f, idx) in items"
                :key="f.task_id + ':' + f.time + ':' + idx"
                class="feedLine"
              >
                <span class="feedTime mono" :title="formatLocalDateTime(f.time)">{{
                  formatLogTime(f.time)
                }}</span>
                <span class="feedTask mono" :title="f.task_id">{{ f.task_short }}</span>
                <span class="feedStream">{{ f.stream }}</span>
                <span class="feedMsg">{{ f.message }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>
  </div>
</template>


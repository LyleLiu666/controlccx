<script setup lang="ts">
import mermaid from "mermaid";
import { computed, nextTick, onMounted, ref, watch } from "vue";
import type { ChatMessage, Task, WorkerType } from "../types";

type SessionGroup = {
  key: string;
  session_id: string;
  deleted_at: string;
  worker_type: WorkerType;
  workdir: string;
  status: Task["status"];
  score: number;
  latest: Task;
};

const props = defineProps<{
  full: boolean;
  width: number;
  resizing: boolean;
  view: "chat" | "overview";
  scope: "current" | "all";
  counts: Record<string, number>;
  needsAttentionSessions: SessionGroup[];
  autopilotEnabled: boolean;
  autopilotNote: string;
  briefing: string;
  chat: ChatMessage[];
  chatBackend: "auto" | "claude" | "codex";
  chatStreamEnabled: boolean;
  chatMaxSteps: number;
  chatStreamStatus: string;
  chatStreamAnswer: string;
  chatSending: boolean;
  chatInput: string;
  theme: "light" | "dark";
  renderMarkdownSafe: (content: string) => string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "startResize", ev: MouseEvent): void;
  (e: "update:full", value: boolean): void;
  (e: "update:view", value: "chat" | "overview"): void;
  (e: "update:scope", value: "current" | "all"): void;
  (e: "update:autopilotEnabled", value: boolean): void;
  (e: "update:chatBackend", value: "auto" | "claude" | "codex"): void;
  (e: "update:chatStreamEnabled", value: boolean): void;
  (e: "update:chatMaxSteps", value: number): void;
  (e: "update:chatInput", value: string): void;
  (e: "selectTask", taskID: string): void;
  (e: "resumeSession", session: SessionGroup): void;
  (e: "cancelSession", session: SessionGroup): void;
  (e: "sendChat"): void;
  (e: "markdownClick", ev: MouseEvent): void;
}>();

const fullModel = computed({
  get: () => props.full,
  set: (value: boolean) => emit("update:full", value),
});
const viewModel = computed({
  get: () => props.view,
  set: (value: "chat" | "overview") => emit("update:view", value),
});
const scopeModel = computed({
  get: () => props.scope,
  set: (value: "current" | "all") => emit("update:scope", value),
});
const autopilotModel = computed({
  get: () => props.autopilotEnabled,
  set: (value: boolean) => emit("update:autopilotEnabled", value),
});
const chatBackendModel = computed({
  get: () => props.chatBackend,
  set: (value: "auto" | "claude" | "codex") => emit("update:chatBackend", value),
});
const chatStreamEnabledModel = computed({
  get: () => props.chatStreamEnabled,
  set: (value: boolean) => emit("update:chatStreamEnabled", value),
});
const chatMaxStepsModel = computed({
  get: () => props.chatMaxSteps,
  set: (value: number) => emit("update:chatMaxSteps", value),
});
const chatInputModel = computed({
  get: () => props.chatInput,
  set: (value: string) => emit("update:chatInput", value),
});

const chatInputEl = ref<HTMLTextAreaElement | null>(null);
const chatMsgsEl = ref<HTMLDivElement | null>(null);
const chatIsComposing = ref(false);

function shortSessionLabel(s: SessionGroup): string {
  const key = String(s?.key ?? "").trim();
  if (key.startsWith("c:")) return key.slice(2, 10);
  return String(s?.session_id || s?.latest?.id || "").slice(0, 8);
}

function shouldAutoScrollChat(el: HTMLElement): boolean {
  const threshold = 90;
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
  return distance <= threshold;
}

function scrollChatToBottom() {
  const el = chatMsgsEl.value;
  if (!el) return;
  try {
    el.scrollTop = el.scrollHeight;
  } catch {
    // ignore
  }
}

function applyMermaidTheme() {
  try {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: props.theme === "dark" ? "dark" : "default",
    });
  } catch {
    // ignore
  }
}

async function renderChatMermaidIfNeeded() {
  if (props.view !== "chat") return;
  const root = chatMsgsEl.value;
  if (!root) return;
  try {
    const nodes = Array.from(root.querySelectorAll<HTMLElement>(".mermaid"));
    if (nodes.length === 0) return;
    await mermaid.run({ nodes });
  } catch {
    // ignore mermaid parse errors
  }
}

async function focusChat() {
  if (props.view !== "chat") return;
  await nextTick();
  chatInputEl.value?.focus();
  scrollChatToBottom();
}

watch(
  () => props.view,
  async (view) => {
    if (view !== "chat") return;
    await focusChat();
  },
  { immediate: false },
);

watch(
  [() => props.view, () => props.theme, () => props.chat.length, () => props.chatStreamAnswer, () => props.chatStreamStatus],
  async ([view]) => {
    if (view !== "chat") return;
    const el = chatMsgsEl.value;
    const stick = el ? shouldAutoScrollChat(el) : true;
    applyMermaidTheme();
    await nextTick();
    await renderChatMermaidIfNeeded();
    if (stick) scrollChatToBottom();
  },
  { immediate: false },
);

onMounted(async () => {
  applyMermaidTheme();
  await focusChat();
  await renderChatMermaidIfNeeded();
  scrollChatToBottom();
});

function onSelectAttentionSession(s: SessionGroup) {
  emit("selectTask", s.latest.id);
  emit("close");
}

function onChatCompositionStart() {
  chatIsComposing.value = true;
}

function onChatCompositionEnd() {
  chatIsComposing.value = false;
}

function isImeComposing(e: KeyboardEvent): boolean {
  return Boolean((e as any).isComposing) || (e as any).keyCode === 229;
}

async function onChatKeyDown(e: KeyboardEvent) {
  if (e.key !== "Enter") return;
  if (e.shiftKey) return; // Shift+Enter -> newline
  if (isImeComposing(e) || chatIsComposing.value) return;
  if (props.chatSending) return;
  if (!props.chatInput.trim()) return;
  e.preventDefault();
  emit("sendChat");
}

function onMarkdownClick(e: MouseEvent) {
  emit("markdownClick", e);
}
</script>

<template>
  <div class="secDrawerOverlay" @click.self="emit('close')">
    <aside
      class="secDrawer secDrawerSecretary"
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
        <div class="secDrawerTitle">Secretary</div>
        <div class="secTabs" role="tablist" aria-label="Secretary tabs">
          <button
            type="button"
            class="secTab"
            :class="{ active: viewModel === 'chat' }"
            role="tab"
            :aria-selected="viewModel === 'chat'"
            @click="viewModel = 'chat'"
          >
            Chat
          </button>
          <button
            type="button"
            class="secTab"
            :class="{ active: viewModel === 'overview' }"
            role="tab"
            :aria-selected="viewModel === 'overview'"
            @click="viewModel = 'overview'"
          >
            Overview
            <span
              v-if="needsAttentionSessions.length"
              class="secTabBadge"
              :title="`Needs attention: ${needsAttentionSessions.length}`"
            >
              {{ needsAttentionSessions.length }}
            </span>
          </button>
        </div>
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
        <div v-if="viewModel === 'overview'" class="secOverview">
          <div class="secretaryCards">
            <div class="secCard">
              <div class="secK">Sessions</div>
              <div class="secV">{{ counts.total }}</div>
            </div>
            <div class="secCard">
              <div class="secK">Running</div>
              <div class="secV">{{ counts.running }}</div>
            </div>
            <div class="secCard">
              <div class="secK">Blocked</div>
              <div class="secV">{{ counts.blocked }}</div>
            </div>
            <div class="secCard">
              <div class="secK">Failed</div>
              <div class="secV">{{ counts.failed }}</div>
            </div>
          </div>

          <div class="secSection">
            <div class="secSectionTitleRow">
              <div class="secSectionTitle">Needs Attention</div>
              <div class="secSectionControls">
                <select v-model="scopeModel" class="secScopeSelect" title="Scope">
                  <option value="current">Current</option>
                  <option value="all">All</option>
                </select>
                <label class="secMiniToggle" title="Auto resume interrupted sessions">
                  <input type="checkbox" v-model="autopilotModel" />
                  Autopilot
                </label>
              </div>
            </div>
            <div v-if="autopilotNote" class="secAutopilotNote">
              {{ autopilotNote }}
            </div>
            <div v-if="needsAttentionSessions.length === 0" class="empty">
              暂无需要关注的 session
            </div>
            <div v-for="s in needsAttentionSessions" :key="s.key" class="secRow">
              <button type="button" class="secRowMain" @click="onSelectAttentionSession(s)">
                <div class="rowTop">
                  <span class="mono">{{ shortSessionLabel(s) }}</span>
                  <span class="pill" :class="s.status">{{ s.status }}</span>
                </div>
                <div class="rowMid">
                  <span class="pill kind">{{ s.worker_type }}</span>
                  <span class="score">score {{ s.score }}</span>
                </div>
                <div class="rowPath mono">{{ s.workdir }}</div>
              </button>
              <div class="secRowActions">
                <button
                  type="button"
                  class="secAction"
                  @click="emit('resumeSession', s)"
                  :disabled="
                    !s.session_id ||
                    !!s.deleted_at ||
                    s.latest.status === 'running' ||
                    s.latest.status === 'queued'
                  "
                  title="Resume session"
                >
                  Resume
                </button>
                <button
                  type="button"
                  class="secAction"
                  @click="emit('cancelSession', s)"
                  :disabled="!(s.latest.status === 'running' || s.latest.status === 'queued')"
                  title="Cancel run"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>

          <div class="secSection">
            <div class="secSectionTitle">Briefing</div>
            <pre class="briefing">{{ briefing }}</pre>
          </div>
        </div>

        <div v-else class="secChatView">
          <div v-if="needsAttentionSessions.length" class="secAttentionHint">
            <div class="text">Needs attention: {{ needsAttentionSessions.length }} session(s)</div>
            <button type="button" @click="viewModel = 'overview'" title="Open overview">
              View
            </button>
          </div>

          <div class="secChat">
            <div class="chat">
              <details class="chatControlsDetails">
                <summary>Chat settings</summary>
                <div class="chatControls">
                  <label>
                    Agent
                    <select v-model="chatBackendModel" :disabled="chatSending">
                      <option value="auto">auto</option>
                      <option value="claude">claude</option>
                      <option value="codex">codex</option>
                    </select>
                  </label>
                  <label class="chatToggle">
                    <input
                      type="checkbox"
                      v-model="chatStreamEnabledModel"
                      :disabled="chatSending"
                    />
                    Stream
                  </label>
                  <label>
                    Max steps
                    <input
                      type="number"
                      min="1"
                      max="32"
                      v-model.number="chatMaxStepsModel"
                      :disabled="chatSending"
                    />
                  </label>
                </div>
              </details>
              <div class="msgs" ref="chatMsgsEl">
                <div v-for="m in chat" :key="m.id" class="msg" :class="m.role">
                  <div class="role">{{ m.role }}</div>
                  <div
                    class="content chatMarkdown"
                    v-html="renderMarkdownSafe(m.content)"
                    @click="onMarkdownClick"
                  ></div>
                </div>
                <div
                  v-if="chatStreamStatus || chatStreamAnswer"
                  class="msg assistant streaming"
                >
                  <div class="role">assistant</div>
                  <div
                    class="content chatMarkdown"
                    v-html="renderMarkdownSafe(chatStreamAnswer || chatStreamStatus)"
                    @click="onMarkdownClick"
                  ></div>
                </div>
              </div>
              <div class="input">
                <textarea
                  ref="chatInputEl"
                  v-model="chatInputModel"
                  rows="3"
                  placeholder="Ask the secretary..."
                  @keydown="onChatKeyDown"
                  @compositionstart="onChatCompositionStart"
                  @compositionend="onChatCompositionEnd"
                ></textarea>
                <button
                  class="primary"
                  @click="emit('sendChat')"
                  :disabled="chatSending || !chatInputModel.trim()"
                >
                  Send
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>
  </div>
</template>

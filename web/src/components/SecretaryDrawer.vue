<script setup lang="ts">
import mermaid from "mermaid";
import { computed, nextTick, onMounted, ref, watch } from "vue";
import type { ChatMessage, PromptTemplate, Task, WorkerType } from "../types";
import { fetchPromptTemplates } from "../api";

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
  secretaryAvailable?: boolean;
  chatBackend: "auto" | "simple-http" | "claude" | "codex";
  chatStreamEnabled: boolean;
  chatMaxSteps: number;
  chatStreamStatus: string;
  chatStreamAnswer: string;
  chatStreamToolError: string;
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
  (e: "update:chatBackend", value: "auto" | "simple-http" | "claude" | "codex"): void;
  (e: "update:chatStreamEnabled", value: boolean): void;
  (e: "update:chatMaxSteps", value: number): void;
  (e: "update:chatInput", value: string): void;
  (e: "selectTask", taskID: string): void;
  (e: "resumeSession", session: SessionGroup): void;
  (e: "cancelSession", session: SessionGroup): void;
  (e: "dismissAttention", session: SessionGroup): void;
  (e: "sendChat"): void;
  (e: "openAuthSettings"): void;
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
  set: (value: "auto" | "simple-http" | "claude" | "codex") => emit("update:chatBackend", value),
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

const secretaryAvailable = computed<boolean>(() => props.secretaryAvailable !== false);

const chatInputEl = ref<HTMLTextAreaElement | null>(null);
const chatMsgsEl = ref<HTMLDivElement | null>(null);
const chatIsComposing = ref(false);

const chatTemplates = ref<PromptTemplate[]>([]);
const chatTemplatesLoading = ref(false);
const chatTemplatesError = ref("");
const selectedChatTemplateID = ref("");

async function loadChatTemplates() {
  if (chatTemplatesLoading.value) return;
  chatTemplatesLoading.value = true;
  chatTemplatesError.value = "";
  try {
    chatTemplates.value = await fetchPromptTemplates("chat");
  } catch (e: any) {
    chatTemplatesError.value = e?.message ?? String(e);
  } finally {
    chatTemplatesLoading.value = false;
  }
}

function applyChatTemplate() {
  const id = String(selectedChatTemplateID.value ?? "").trim();
  if (!id) return;
  const tpl = chatTemplates.value.find((t) => t.id === id);
  if (!tpl) return;
  chatInputModel.value = String(tpl.content ?? "");
  void focusChat();
}

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
  void loadChatTemplates();
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
          class="secHeaderAction"
          type="button"
          @click="emit('openAuthSettings')"
          title="打开秘书 LLM 认证设置"
        >
          秘书设置
        </button>
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
              <div class="secK">会话</div>
              <div class="secV">{{ counts.total }}</div>
            </div>
            <div class="secCard">
              <div class="secK">运行中</div>
              <div class="secV">{{ counts.running }}</div>
            </div>
            <div class="secCard">
              <div class="secK">阻塞</div>
              <div class="secV">{{ counts.blocked }}</div>
            </div>
            <div class="secCard">
              <div class="secK">失败</div>
              <div class="secV">{{ counts.failed }}</div>
            </div>
          </div>

          <div class="secSection secSectionAttention">
            <div class="secSectionTitleRow">
              <div class="secSectionHead">
                <div class="secSectionTitle">需要关注</div>
                <div class="secSectionSubtitle">阻塞 / 中断会话会显示在这里</div>
              </div>
              <div class="secSectionControls">
                <select v-model="scopeModel" class="secScopeSelect" title="范围">
                  <option value="current">当前</option>
                  <option value="all">全部</option>
                </select>
                <label class="secMiniToggle" title="自动尝试继续中断的会话">
                  <input type="checkbox" v-model="autopilotModel" />
                  自动
                </label>
              </div>
            </div>
            <div v-if="autopilotNote" class="secAutopilotNote">
              {{ autopilotNote }}
            </div>
            <div v-if="needsAttentionSessions.length === 0" class="secEmptyState">
              <div class="secEmptyTitle">当前没有需要处理的会话</div>
              <div class="secEmptyHint">出现 blocked / interrupted 后会自动显示在这里。</div>
            </div>
            <div v-else class="secRows">
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
                      s.latest.status === 'queued' ||
                      s.latest.status === 'waiting'
                    "
                    title="继续会话"
                  >
                    继续
                  </button>
                  <button
                    v-if="
                      s.latest.status === 'running' ||
                      s.latest.status === 'queued' ||
                      s.latest.status === 'waiting'
                    "
                    type="button"
                    class="secAction"
                    @click="emit('cancelSession', s)"
                    title="取消运行"
                  >
                    取消运行
                  </button>
                  <button
                    v-else
                    type="button"
                    class="secAction"
                    @click="emit('dismissAttention', s)"
                    title="不再提示"
                  >
                    取消提醒
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="secSection secSectionBriefing">
            <div class="secSectionHead">
              <div class="secSectionTitle">简报</div>
              <div class="secSectionSubtitle">当前范围的会话状态快照</div>
            </div>
            <pre class="briefing">{{ briefing }}</pre>
          </div>
        </div>

        <div v-else class="secChatView">
          <div v-if="needsAttentionSessions.length" class="secAttentionHint">
            <div class="text">需要关注：{{ needsAttentionSessions.length }} 个会话</div>
            <button type="button" @click="viewModel = 'overview'" title="打开概览">
              查看
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
                      <option value="simple-http">simple-http</option>
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
                <div class="chatSettingsHint">
                  Agent 仅切换后端；模型和 token 在
                  <button type="button" class="secInlineLink" @click="emit('openAuthSettings')">
                    认证设置
                  </button>
                  中配置。
                </div>
              </details>
              <div class="msgs" ref="chatMsgsEl">
                <div v-if="chatStreamToolError" class="secStreamToolError">
                  {{ chatStreamToolError }}
                </div>
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
              <div class="secChatTemplatesRow">
                <span class="tinyHint">模板</span>
                <select
                  v-model="selectedChatTemplateID"
                  :disabled="chatTemplatesLoading || !chatTemplates.length"
                  title="选择一个 chat 模板并应用到输入框"
                >
                  <option value="">(选择对话模板…)</option>
                  <option v-for="t in chatTemplates" :key="t.id" :value="t.id">
                    {{ t.title }}
                  </option>
                </select>
                <button
                  type="button"
                  @click="applyChatTemplate"
                  :disabled="chatTemplatesLoading || !selectedChatTemplateID"
                >
                  应用
                </button>
                <button
                  type="button"
                  @click="loadChatTemplates"
                  :disabled="chatTemplatesLoading"
                  title="刷新模板列表"
                >
                  刷新
                </button>
                <span v-if="chatTemplatesLoading" class="tinyHint">加载中…</span>
              </div>
              <div v-if="chatTemplatesError" class="tinyHint warn">{{ chatTemplatesError }}</div>
              <div class="tinyHint">
                Project Context（如已设置）会自动注入到对话（压缩/限长）。
              </div>
              <div v-if="!secretaryAvailable" class="secDegradedHint" role="note">
                <div class="text">秘书守护进程不可用：对话已禁用。</div>
                <button type="button" class="secInlineLink" @click="emit('openAuthSettings')">
                  认证设置
                </button>
              </div>
              <div class="input">
                <textarea
                  ref="chatInputEl"
                  v-model="chatInputModel"
                  rows="3"
                  :disabled="!secretaryAvailable"
                  :placeholder="secretaryAvailable ? 'Ask the secretary...' : 'Secretary daemon unavailable'"
                  @keydown="onChatKeyDown"
                  @compositionstart="onChatCompositionStart"
                  @compositionend="onChatCompositionEnd"
                ></textarea>
                <button
                  class="primary"
                  @click="emit('sendChat')"
                  :disabled="!secretaryAvailable || chatSending || !chatInputModel.trim()"
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

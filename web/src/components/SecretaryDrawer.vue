<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import type { SecretaryMessage } from "../types";

const props = defineProps<{
  messages: SecretaryMessage[];
  loading: boolean;
  sending: boolean;
  error: string;
  input: string;
  thinkingLines: string[];
  streamingReply: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "refresh"): void;
  (e: "clear"): void;
  (e: "send"): void;
  (e: "quick", message: string): void;
  (e: "update:input", value: string): void;
}>();

const inputModel = computed({
  get: () => props.input,
  set: (v: string) => emit("update:input", v),
});

const listEl = ref<HTMLDivElement | null>(null);
const thinkingEl = ref<HTMLDivElement | null>(null);

function scrollToBottom() {
  const el = listEl.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
}

function scrollThinkingToBottom() {
  const el = thinkingEl.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
}

watch(
  () => props.messages.length,
  async () => {
    await nextTick();
    scrollToBottom();
  },
);

watch(
  () => props.thinkingLines.length,
  async () => {
    await nextTick();
    scrollThinkingToBottom();
  },
);

watch(
  () => props.streamingReply,
  async () => {
    await nextTick();
    scrollToBottom();
  },
);

function onInputKeydown(ev: KeyboardEvent) {
  if (ev.key !== "Enter") return;
  if (ev.shiftKey) return;
  ev.preventDefault();
  emit("send");
}

const quickPrompts = [
  "现在一共有多少个任务？",
  "已完成（succeeded）的有多少？",
  "失败（failed）的有多少？",
];
</script>

<template>
  <div class="secDrawerOverlay" @click.self="emit('close')">
    <aside class="secDrawer wide" role="dialog" aria-modal="true">
      <div class="secDrawerHeader">
        <div class="secDrawerTitle">秘书</div>
        <div class="spacer"></div>
        <button type="button" class="secHeaderAction" @click="emit('refresh')" :disabled="loading || sending">
          刷新
        </button>
        <button type="button" class="secHeaderAction" @click="emit('clear')" :disabled="loading || sending">
          清空
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="secDrawerBody">
        <div v-if="error" class="tinyHint warn mono">{{ error }}</div>

        <div ref="listEl" class="secChatList" role="log" aria-label="Secretary chat">
          <div v-if="loading && messages.length === 0" class="tinyHint">加载中…</div>
          <div v-else-if="messages.length === 0 && !streamingReply && !sending" class="tinyHint">还没有对话，试着问一句。</div>
          <div v-else class="secChatItems">
            <div
              v-for="m in messages"
              :key="m.id"
              class="secChatItem"
              :class="m.role === 'user' ? 'user' : 'assistant'"
            >
              <div class="secChatBubble">
                <div class="secChatRole">{{ m.role === "user" ? "你" : "秘书" }}</div>
                <div class="secChatText">{{ m.content }}</div>
              </div>
            </div>
            <div v-if="sending || streamingReply" class="secChatItem assistant">
              <div class="secChatBubble">
                <div class="secChatRole">秘书</div>
                <div class="secChatText">{{ streamingReply || "思考中…" }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="secChatQuick">
          <button
            v-for="q in quickPrompts"
            :key="q"
            type="button"
            class="secChatQuickBtn"
            @click="emit('quick', q)"
            :disabled="loading || sending"
          >
            {{ q }}
          </button>
        </div>

        <div class="secThinkingPanel">
          <div class="secThinkingHead">思考与工具过程（固定 3 行窗口）</div>
          <div ref="thinkingEl" class="secThinkingViewport mono">
            <div v-if="thinkingLines.length === 0" class="tinyHint">等待思考过程流式输出…</div>
            <div v-for="(line, idx) in thinkingLines" :key="`${idx}-${line}`" class="secThinkingLine">
              {{ line }}
            </div>
          </div>
        </div>

        <div class="secChatComposer">
          <textarea
            v-model="inputModel"
            class="secChatInput"
            rows="2"
            placeholder="用自然语言问我：比如“失败的任务有多少？”"
            @keydown="onInputKeydown"
            :disabled="loading"
          ></textarea>
          <button type="button" class="primary" @click="emit('send')" :disabled="sending || !inputModel.trim()">
            {{ sending ? "发送中…" : "发送" }}
          </button>
        </div>
      </div>
    </aside>
  </div>
</template>

<style scoped>
.spacer {
  flex: 1;
}

.secChatList {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: rgba(15, 23, 42, 0.08);
  display: grid;
  gap: 10px;
}

.secChatItems {
  display: grid;
  gap: 10px;
}

.secChatItem {
  display: flex;
}

.secChatItem.user {
  justify-content: flex-end;
}

.secChatBubble {
  max-width: min(780px, 92%);
  border-radius: 14px;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  display: grid;
  gap: 6px;
}

.secChatItem.user .secChatBubble {
  border-color: rgba(56, 189, 248, 0.5);
  background: rgba(56, 189, 248, 0.08);
}

.secChatRole {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
}

.secChatText {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  color: var(--text-main);
}

.secChatQuick {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.secChatQuickBtn {
  font-size: 12px;
  padding: 8px 10px;
  border-radius: 999px;
  border: 1px solid var(--border-color);
  background: var(--bg-subtle);
  color: var(--text-main);
}

.secChatQuickBtn:hover:not(:disabled) {
  border-color: rgba(56, 189, 248, 0.6);
}

.secChatComposer {
  margin-top: 10px;
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
  align-items: end;
}

.secChatInput {
  resize: vertical;
  min-height: 44px;
}

.secThinkingPanel {
  --sec-thinking-lines: 3;
  margin-top: 10px;
  padding: 10px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: rgba(15, 23, 42, 0.08);
  display: grid;
  gap: 8px;
}

.secThinkingHead {
  font-size: 12px;
  color: var(--text-sub);
  font-weight: 700;
}

.secThinkingViewport {
  max-height: calc(var(--sec-thinking-lines) * 1.45em + 10px);
  height: calc(var(--sec-thinking-lines) * 1.45em + 10px);
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 2px;
}

.secThinkingLine {
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-main);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>

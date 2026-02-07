import { ref } from "vue";
import type { ChatMessage } from "../types";
import { fetchChat, sendChatStream } from "../api";
import { buildChatIdempotencyKey } from "../chatOps";

function lastChatID(list: ChatMessage[]): number {
  if (!Array.isArray(list) || list.length === 0) return 0;
  const last = list[list.length - 1];
  return typeof last?.id === "number" && Number.isFinite(last.id) ? last.id : 0;
}

function mergeChatMessages(current: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  if (!Array.isArray(incoming) || incoming.length === 0) return current;
  const seen = new Set<number>();
  for (const m of current) {
    if (m && typeof m.id === "number") seen.add(m.id);
  }
  const added = incoming.filter((m) => m && typeof m.id === "number" && !seen.has(m.id));
  return added.length ? [...current, ...added] : current;
}

export function useSecretaryChat(opts?: { onError?: (message: string) => void }) {
  const onError = opts?.onError ?? (() => {});

  const chat = ref<ChatMessage[]>([]);
  const chatInput = ref<string>("");
  const chatBackend = ref<"auto" | "simple-http" | "claude" | "codex">("auto");
  const chatMaxSteps = ref<number>(8);
  const chatStreamStatus = ref<string>("");
  const chatStreamAnswer = ref<string>("");
  const chatStreamToolError = ref<string>("");
  const chatSendError = ref<string>("");
  const chatSending = ref<boolean>(false);

  async function refreshChat() {
    try {
      const after = lastChatID(chat.value);
      const incoming = await fetchChat(after, 200);
      if (!after) {
        chat.value = incoming;
        return;
      }
      chat.value = mergeChatMessages(chat.value, incoming);
    } catch (e: any) {
      onError(e?.message ?? String(e));
    }
  }

  async function syncChatFrom(after: number) {
    const incoming = await fetchChat(Math.max(0, after || 0), 200);
    chat.value = mergeChatMessages(chat.value, incoming);
  }

  async function sendChatMessage() {
    const rawInput = chatInput.value;
    const msg = rawInput.trim();
    if (!msg) return;
    if (chatSending.value) return;

    const after = lastChatID(chat.value);
    const idempotencyKey = buildChatIdempotencyKey({
      after,
      message: msg,
      backend: chatBackend.value,
      maxSteps: chatMaxSteps.value,
    });

    onError("");
    chatSendError.value = "";
    chatSending.value = true;
    try {
      chatStreamStatus.value = "thinking";
      chatStreamAnswer.value = "";
      chatStreamToolError.value = "";
      chatInput.value = "";

      await sendChatStream(
        msg,
        { backend: chatBackend.value, max_steps: chatMaxSteps.value, idempotency_key: idempotencyKey },
        (evt) => {
          if (evt.event === "status") {
            const phase = String(evt.data?.phase ?? "");
            if (phase) chatStreamStatus.value = phase;
            return;
          }
          if (evt.event === "tool_call") {
            const tool = String(evt.data?.tool ?? "");
            if (tool) chatStreamStatus.value = `tool: ${tool}`;
            return;
          }
          if (evt.event === "tool_result") {
            const tool = String(evt.data?.tool ?? "");
            const result = evt.data?.result;
            if (result && typeof result === "object" && result.ok === false) {
              const detail = String(result.error ?? "").trim() || "tool call failed";
              chatStreamToolError.value = tool
                ? `tool ${tool} error: ${detail}`
                : `tool error: ${detail}`;
              chatStreamStatus.value = tool ? `tool failed: ${tool}` : "tool failed";
              return;
            }
            if (tool) chatStreamStatus.value = `tool done: ${tool}`;
            return;
          }
          if (evt.event === "final") {
            const m = String(evt.data?.message ?? "");
            if (m) chatStreamAnswer.value = m;
            chatStreamStatus.value = "";
          }
        },
      );

      await syncChatFrom(after);
      chatStreamStatus.value = "";
      chatStreamAnswer.value = "";
      chatStreamToolError.value = "";
    } catch (e: any) {
      const message = e?.message ?? String(e);
      chatSendError.value = message;
      onError(message);
      chatStreamStatus.value = "";
      chatInput.value = rawInput;
    } finally {
      chatSending.value = false;
    }
  }

  return {
    chat,
    chatInput,
    chatBackend,
    chatMaxSteps,
    chatStreamStatus,
    chatStreamAnswer,
    chatStreamToolError,
    chatSendError,
    chatSending,
    refreshChat,
    syncChatFrom,
    sendChatMessage,
  };
}

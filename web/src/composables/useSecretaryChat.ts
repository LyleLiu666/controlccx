import { nextTick, ref } from "vue";
import type { SecretaryMessage, SecretaryThinkingEvent } from "../types";
import { clearSecretaryMessages, fetchSecretaryMessages, sendSecretaryMessageStream } from "../api";

export function useSecretaryChat() {
  const open = ref(false);
  const messages = ref<SecretaryMessage[]>([]);
  const input = ref("");
  const loading = ref(false);
  const sending = ref(false);
  const error = ref("");
  const thinkingLines = ref<string[]>([]);
  const streamingReply = ref("");

  async function refresh(limit = 200) {
    if (loading.value) return;
    loading.value = true;
    try {
      messages.value = await fetchSecretaryMessages(limit);
      error.value = "";
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  async function openDrawer() {
    open.value = true;
    await refresh(200);
    await nextTick();
  }

  function closeDrawer() {
    open.value = false;
  }

  function appendThinkingLine(line: string) {
    const v = String(line ?? "").trim();
    if (!v) return;
    const next = [...thinkingLines.value, v];
    const keep = 300;
    thinkingLines.value = next.length > keep ? next.slice(next.length - keep) : next;
  }

  function formatThinkingLine(event: SecretaryThinkingEvent): string {
    const step = Number.isFinite(event?.step) ? `#${Number(event?.step)} ` : "";
    const kind = String(event?.kind ?? "").trim();
    const line = String(event?.line ?? "").trim();
    const tool = String(event?.tool_name ?? "").trim();
    if (kind === "tool_call") {
      return step + (line || `调用工具：${tool || "unknown"}`);
    }
    if (kind === "tool_result") {
      return step + (line || `工具完成：${tool || "unknown"}`);
    }
    if (kind === "error") {
      const err = String(event?.error ?? "").trim();
      return step + (line || err || "发生错误");
    }
    return step + line;
  }

  async function send(opts?: { message?: string; refresh?: boolean }) {
    if (sending.value) return;
    const msg = String(opts?.message ?? input.value ?? "").trim();
    if (!msg) return;
    sending.value = true;
    error.value = "";
    input.value = "";
    thinkingLines.value = [];
    streamingReply.value = "";

    const now = new Date().toISOString();
    const userID = Date.now();
    messages.value = [
      ...messages.value,
      { id: userID, time: now, role: "user", content: msg },
    ];

    try {
      const res = await sendSecretaryMessageStream(msg, {
        onDelta: (delta: string) => {
          streamingReply.value += String(delta ?? "");
        },
        onThinking: (event: SecretaryThinkingEvent) => {
          appendThinkingLine(formatThinkingLine(event));
        },
        onDone: (reply: string) => {
          if (!streamingReply.value.trim()) {
            streamingReply.value = String(reply ?? "");
          }
        },
      });

      let finalReply = String(res?.reply ?? "").trim();
      if (!finalReply) {
        finalReply = String(streamingReply.value ?? "").trim();
      }
      if (!finalReply) {
        finalReply = "秘书没有返回内容，请重试。";
      }
      messages.value = [
        ...messages.value,
        {
          id: Date.now() + 1,
          time: new Date().toISOString(),
          role: "assistant",
          content: finalReply,
        },
      ];
    } catch (e: any) {
      error.value = e?.message ?? String(e);
      const partial = String(streamingReply.value ?? "").trim();
      if (partial) {
        messages.value = [
          ...messages.value,
          {
            id: Date.now() + 1,
            time: new Date().toISOString(),
            role: "assistant",
            content: partial,
          },
        ];
      }
    } finally {
      streamingReply.value = "";
      sending.value = false;
    }
  }

  async function clear() {
    if (loading.value) return;
    loading.value = true;
    error.value = "";
    try {
      await clearSecretaryMessages();
      messages.value = [];
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  return {
    open,
    messages,
    input,
    loading,
    sending,
    error,
    thinkingLines,
    streamingReply,
    refresh,
    openDrawer,
    closeDrawer,
    send,
    clear,
  };
}

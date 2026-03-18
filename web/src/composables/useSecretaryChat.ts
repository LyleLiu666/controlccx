import { nextTick, onBeforeUnmount, ref } from "vue";
import type { SecretaryMessage, SecretaryMessageRole, SecretaryThinkingEvent, ServerEvent } from "../types";
import { clearSecretaryMessages, fetchSecretaryMessages, sendSecretaryMessageStream } from "../api";

type UseSecretaryChatOptions = {
  getConversationID?: () => string;
};

export function useSecretaryChat(opts?: UseSecretaryChatOptions) {
  const autoRefreshMs = 2500;
  const open = ref(false);
  const messages = ref<SecretaryMessage[]>([]);
  const input = ref("");
  const loading = ref(false);
  const sending = ref(false);
  const error = ref("");
  const thinkingLines = ref<string[]>([]);
  const streamingReply = ref("");
  const conversationID = ref("");
  let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;

  function resolveConversationID() {
    const fromOpts = String(opts?.getConversationID?.() ?? "").trim();
    if (fromOpts) return fromOpts;
    const local = String(conversationID.value ?? "").trim();
    if (local) return local;
    return "__global__";
  }

  async function refresh(limit = 200) {
    if (loading.value) return;
    loading.value = true;
    try {
      messages.value = await fetchSecretaryMessages(limit, resolveConversationID());
      error.value = "";
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  async function refreshSilently(limit = 200) {
    try {
      messages.value = await fetchSecretaryMessages(limit, resolveConversationID());
      error.value = "";
    } catch {
      // Keep current UI state on background refresh failures.
    }
  }

  function startAutoRefresh() {
    if (autoRefreshTimer) return;
    autoRefreshTimer = setInterval(() => {
      if (!open.value || sending.value || loading.value) return;
      void refreshSilently(200);
    }, autoRefreshMs);
  }

  function stopAutoRefresh() {
    if (!autoRefreshTimer) return;
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }

  async function openDrawer() {
    open.value = true;
    await refresh(200);
    startAutoRefresh();
    await nextTick();
  }

  function closeDrawer() {
    open.value = false;
    stopAutoRefresh();
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

  function handleServerEvent(evt: ServerEvent) {
    if (!evt || typeof evt.type !== "string") return;
    if (evt.type === "secretary.thinking") {
      const payload = (evt.payload ?? {}) as Record<string, any>;
      const eventConversationID = String(payload.conversation_id ?? "").trim();
      if (eventConversationID && eventConversationID !== resolveConversationID()) {
        return;
      }
      const thinking: SecretaryThinkingEvent = {
        kind: typeof payload.kind === "string" ? payload.kind : undefined,
        step: Number.isFinite(payload.step) ? Number(payload.step) : undefined,
        line: typeof payload.line === "string" ? payload.line : "",
        tool_name: typeof payload.tool_name === "string" ? payload.tool_name : undefined,
        ok: typeof payload.ok === "boolean" ? payload.ok : undefined,
        error: typeof payload.error === "string" ? payload.error : undefined,
      };
      appendThinkingLine(formatThinkingLine(thinking));
      return;
    }
    if (evt.type !== "secretary.message") return;

    const payload = (evt.payload ?? {}) as Record<string, any>;
    const eventConversationID = String(payload.conversation_id ?? "").trim();
    if (eventConversationID && eventConversationID !== resolveConversationID()) {
      return;
    }
    const content =
      typeof payload.content === "string"
        ? payload.content.trim()
        : typeof payload.message === "string"
          ? payload.message.trim()
          : "";
    if (!content) return;

    const roleRaw = typeof payload.role === "string" ? payload.role.trim() : "";
    const role: SecretaryMessageRole = roleRaw === "user" ? "user" : "assistant";
    const id = Number.isFinite(payload.id) ? Number(payload.id) : Date.now();
    const timeValue = typeof payload.time === "string" && payload.time.trim() ? payload.time : new Date().toISOString();
    messages.value = [
      ...messages.value,
      {
        id,
        time: timeValue,
        role,
        content,
      },
    ];
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
      }, { conversationID: resolveConversationID() });

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
      await clearSecretaryMessages(resolveConversationID());
      messages.value = [];
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  onBeforeUnmount(() => {
    stopAutoRefresh();
  });

  return {
    open,
    messages,
    conversationID,
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
    handleServerEvent,
  };
}

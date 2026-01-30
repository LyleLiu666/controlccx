import { ref } from "vue";
import type { ChatMessage } from "../types";
import { fetchChat, sendChat, sendChatStream } from "../api";
import { sendChatAndReload } from "../chatOps";

export function useSecretaryChat(opts?: { onError?: (message: string) => void }) {
  const onError = opts?.onError ?? (() => {});

  const chat = ref<ChatMessage[]>([]);
  const chatInput = ref<string>("");
  const chatBackend = ref<"auto" | "claude" | "codex">("auto");
  const chatStreamEnabled = ref<boolean>(true);
  const chatMaxSteps = ref<number>(8);
  const chatStreamStatus = ref<string>("");
  const chatStreamAnswer = ref<string>("");
  const chatSending = ref<boolean>(false);

  async function refreshChat() {
    try {
      chat.value = await fetchChat();
    } catch (e: any) {
      onError(e?.message ?? String(e));
    }
  }

  async function sendChatMessage() {
    const msg = chatInput.value.trim();
    if (!msg) return;
    if (chatSending.value) return;

    onError("");
    chatSending.value = true;
    try {
      if (!chatStreamEnabled.value) {
        chat.value = await sendChatAndReload(msg, { sendChat, fetchChat });
        chatInput.value = "";
        return;
      }

      chatStreamStatus.value = "thinking";
      chatStreamAnswer.value = "";
      chatInput.value = "";

      await sendChatStream(
        msg,
        { backend: chatBackend.value, max_steps: chatMaxSteps.value },
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

      chat.value = await fetchChat();
      chatStreamStatus.value = "";
      chatStreamAnswer.value = "";
    } catch (e: any) {
      onError(e?.message ?? String(e));
    } finally {
      chatSending.value = false;
    }
  }

  return {
    chat,
    chatInput,
    chatBackend,
    chatStreamEnabled,
    chatMaxSteps,
    chatStreamStatus,
    chatStreamAnswer,
    chatSending,
    refreshChat,
    sendChatMessage,
  };
}


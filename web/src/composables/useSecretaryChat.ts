import { nextTick, ref } from "vue";
import type { SecretaryMessage } from "../types";
import { clearSecretaryMessages, fetchSecretaryMessages, sendSecretaryMessage } from "../api";

export function useSecretaryChat() {
  const open = ref(false);
  const messages = ref<SecretaryMessage[]>([]);
  const input = ref("");
  const loading = ref(false);
  const sending = ref(false);
  const error = ref("");

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

  async function send(opts?: { message?: string; refresh?: boolean }) {
    if (sending.value) return;
    const msg = String(opts?.message ?? input.value ?? "").trim();
    if (!msg) return;
    sending.value = true;
    error.value = "";
    try {
      const res = await sendSecretaryMessage(msg);
      input.value = "";
      if (opts?.refresh ?? true) {
        await refresh(200);
      } else {
        // Best-effort: append locally if caller opts out of refresh.
        messages.value = [
          ...messages.value,
          { id: Date.now(), time: new Date().toISOString(), role: "user", content: msg },
          {
            id: Date.now() + 1,
            time: new Date().toISOString(),
            role: "assistant",
            content: String(res?.reply ?? ""),
          },
        ];
      }
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
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
    refresh,
    openDrawer,
    closeDrawer,
    send,
    clear,
  };
}


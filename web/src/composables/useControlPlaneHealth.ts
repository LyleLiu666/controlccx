import { computed, onMounted, onUnmounted, ref } from "vue";
import type { ControlPlaneStatus } from "../types";
import { fetchControlPlaneStatus } from "../api";

export function useControlPlaneHealth(opts?: {
  intervalMs?: number;
  onError?: (message: string) => void;
}) {
  const intervalMs = Math.max(500, Number(opts?.intervalMs ?? 2000));
  const onError = opts?.onError ?? (() => {});

  const status = ref<ControlPlaneStatus | null>(null);
  const error = ref<string>("");
  const loading = ref<boolean>(false);

  const runnerOK = computed<boolean>(() => {
    const s = status.value;
    if (!s) return true;
    return !!s.runnerd?.ok;
  });
  const secretaryOK = computed<boolean>(() => {
    const s = status.value;
    if (!s) return true;
    return !!s.secretaryd?.ok;
  });

  async function refresh() {
    if (loading.value) return;
    loading.value = true;
    try {
      const next = await fetchControlPlaneStatus();
      status.value = next;
      error.value = "";
      onError("");
    } catch (e: any) {
      const msg = e?.message ?? String(e);
      error.value = msg;
      onError(msg);
    } finally {
      loading.value = false;
    }
  }

  let timer: number | null = null;
  onMounted(() => {
    void refresh();
    timer = window.setInterval(() => {
      void refresh();
    }, intervalMs);
  });
  onUnmounted(() => {
    if (timer != null) window.clearInterval(timer);
    timer = null;
  });

  return {
    status,
    error,
    loading,
    runnerOK,
    secretaryOK,
    refresh,
  };
}


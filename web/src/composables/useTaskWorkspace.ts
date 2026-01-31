import { ref, watch, type Ref } from "vue";
import type { ConflictError, SessionWorkspace } from "../types";
import { discardTaskWorkspace, fetchTaskWorkspace, mergeTaskWorkspace } from "../api";

export function useTaskWorkspace(taskId: Ref<string>) {
  const workspace = ref<SessionWorkspace | null>(null);
  const loading = ref(false);
  const error = ref("");
  const conflict = ref<ConflictError | null>(null);

  async function refresh() {
    const id = taskId.value.trim();
    error.value = "";
    conflict.value = null;
    if (!id) {
      workspace.value = null;
      return;
    }
    loading.value = true;
    try {
      workspace.value = await fetchTaskWorkspace(id);
    } catch (e: any) {
      error.value = e?.message ?? String(e);
      workspace.value = null;
    } finally {
      loading.value = false;
    }
  }

  async function merge() {
    const id = taskId.value.trim();
    if (!id) return;
    error.value = "";
    conflict.value = null;
    loading.value = true;
    try {
      const res = await mergeTaskWorkspace(id);
      if (res.ok) {
        await refresh();
        return;
      }
      conflict.value = res.conflict;
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  async function discard() {
    const id = taskId.value.trim();
    if (!id) return;
    error.value = "";
    conflict.value = null;
    loading.value = true;
    try {
      await discardTaskWorkspace(id);
      await refresh();
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  watch(taskId, () => {
    void refresh();
  }, { immediate: true });

  return {
    workspace,
    loading,
    error,
    conflict,
    refresh,
    merge,
    discard,
  };
}


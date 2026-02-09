import { computed, ref, watch, type Ref } from "vue";
import type { SessionWorkspace, SessionWorkspaceMergeResponse } from "../types";
import { discardSessionWorkspace, ensureSessionWorkspace, fetchSessionWorkspace, mergeSessionWorkspace } from "../api";

export function useSessionWorkspace(selectedKey: Ref<string>) {
  const workspaceByKey = ref<Map<string, SessionWorkspace | null>>(new Map());
  const workspaceLoading = ref(false);
  const workspaceError = ref("");

  const selectedWorkspace = computed(() => {
    const key = String(selectedKey.value ?? "").trim();
    if (!key) return null;
    return workspaceByKey.value.get(key) ?? null;
  });

  async function loadWorkspace(key: string, opts?: { force?: boolean }) {
    key = String(key ?? "").trim();
    if (!key) return;
    if (!opts?.force && workspaceByKey.value.has(key)) return;

    workspaceLoading.value = true;
    workspaceError.value = "";
    try {
      const res = await fetchSessionWorkspace(key);
      const next = new Map(workspaceByKey.value);
      next.set(key, res.ok ? (res.workspace ?? null) : null);
      workspaceByKey.value = next;
    } catch (e: any) {
      workspaceError.value = e?.message ?? String(e);
    } finally {
      workspaceLoading.value = false;
    }
  }

  async function mergeWorkspace(key: string): Promise<SessionWorkspaceMergeResponse | null> {
    key = String(key ?? "").trim();
    if (!key) return null;
    workspaceLoading.value = true;
    workspaceError.value = "";
    try {
      const res = await mergeSessionWorkspace(key);
      const next = new Map(workspaceByKey.value);
      next.set(key, res.workspace ?? null);
      workspaceByKey.value = next;
      return res;
    } catch (e: any) {
      workspaceError.value = e?.message ?? String(e);
      return null;
    } finally {
      workspaceLoading.value = false;
    }
  }

  async function ensureWorkspace(key: string): Promise<SessionWorkspace | null> {
    key = String(key ?? "").trim();
    if (!key) return null;
    workspaceLoading.value = true;
    workspaceError.value = "";
    try {
      const res = await ensureSessionWorkspace(key);
      const next = new Map(workspaceByKey.value);
      next.set(key, res.ok ? (res.workspace ?? null) : null);
      workspaceByKey.value = next;
      return res.ok ? (res.workspace ?? null) : null;
    } catch (e: any) {
      workspaceError.value = e?.message ?? String(e);
      return null;
    } finally {
      workspaceLoading.value = false;
    }
  }

  async function discardWorkspace(key: string): Promise<boolean> {
    key = String(key ?? "").trim();
    if (!key) return false;
    workspaceLoading.value = true;
    workspaceError.value = "";
    try {
      const res = await discardSessionWorkspace(key);
      await loadWorkspace(key, { force: true });
      return Boolean(res.ok);
    } catch (e: any) {
      workspaceError.value = e?.message ?? String(e);
      return false;
    } finally {
      workspaceLoading.value = false;
    }
  }

  watch(
    selectedKey,
    (k) => {
      const key = String(k ?? "").trim();
      if (!key) return;
      void loadWorkspace(key);
    },
    { immediate: true },
  );

  return {
    workspaceByKey,
    workspaceLoading,
    workspaceError,
    selectedWorkspace,
    loadWorkspace,
    ensureWorkspace,
    mergeWorkspace,
    discardWorkspace,
  };
}

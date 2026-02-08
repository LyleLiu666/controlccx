import { ref } from "vue";
import {
  fetchAuditEntries,
  fetchAuditEntry,
  fetchAuditRetention,
  fetchAuditSources,
} from "../api";
import type {
  AuditEntry,
  AuditEntryDetail,
  AuditQuery,
  AuditRetentionStatus,
  AuditSource,
  AuditSourceInfo,
} from "../types";

const DEFAULT_STREAMS: Array<"stdout" | "stderr" | "system" | "assistant"> = [
  "stdout",
  "stderr",
  "system",
  "assistant",
];

function localDateTimeToISO(value: string): string {
  const s = String(value ?? "").trim();
  if (!s) return "";
  const date = new Date(s);
  if (Number.isNaN(date.getTime())) return "";
  return date.toISOString();
}

export function useAudit() {
  const loading = ref(false);
  const loadingMore = ref(false);
  const loadingDetail = ref(false);
  const loadingMeta = ref(false);
  const error = ref("");

  const sources = ref<AuditSourceInfo[]>([]);
  const retention = ref<AuditRetentionStatus | null>(null);

  const entries = ref<AuditEntry[]>([]);
  const nextCursor = ref("");
  const selectedID = ref("");
  const detail = ref<AuditEntryDetail | null>(null);

  const querySources = ref<AuditSource[]>([]);
  const queryKeyword = ref("");
  const queryFrom = ref("");
  const queryTo = ref("");
  const queryTaskID = ref("");
  const queryRunID = ref("");
  const queryStreams = ref<Array<"stdout" | "stderr" | "system" | "assistant">>([
    ...DEFAULT_STREAMS,
  ]);

  function buildQuery(cursor?: string): AuditQuery {
    return {
      sources: querySources.value.slice(),
      q: String(queryKeyword.value ?? "").trim(),
      from: localDateTimeToISO(queryFrom.value),
      to: localDateTimeToISO(queryTo.value),
      task_id: String(queryTaskID.value ?? "").trim(),
      run_id: String(queryRunID.value ?? "").trim(),
      streams: queryStreams.value.slice(),
      limit: 100,
      cursor: String(cursor ?? "").trim(),
    };
  }

  async function refreshMeta() {
    loadingMeta.value = true;
    try {
      const [srcRes, retentionRes] = await Promise.all([
        fetchAuditSources(),
        fetchAuditRetention(),
      ]);
      sources.value = Array.isArray(srcRes?.sources) ? srcRes.sources : [];
      retention.value = retentionRes ?? null;
    } finally {
      loadingMeta.value = false;
    }
  }

  async function search() {
    if (loading.value) return;
    loading.value = true;
    error.value = "";
    try {
      const res = await fetchAuditEntries(buildQuery(""));
      entries.value = Array.isArray(res?.entries) ? res.entries : [];
      nextCursor.value = String(res?.next_cursor ?? "");
      const firstID = entries.value[0]?.id ?? "";
      selectedID.value = firstID;
      if (firstID) {
        await selectEntry(firstID);
      } else {
        detail.value = null;
      }
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  async function loadMore() {
    const cursor = String(nextCursor.value ?? "").trim();
    if (!cursor || loadingMore.value) return;
    loadingMore.value = true;
    error.value = "";
    try {
      const res = await fetchAuditEntries(buildQuery(cursor));
      const list = Array.isArray(res?.entries) ? res.entries : [];
      entries.value = [...entries.value, ...list];
      nextCursor.value = String(res?.next_cursor ?? "");
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loadingMore.value = false;
    }
  }

  async function selectEntry(id: string) {
    const entryID = String(id ?? "").trim();
    if (!entryID || loadingDetail.value) return;
    loadingDetail.value = true;
    selectedID.value = entryID;
    error.value = "";
    try {
      detail.value = await fetchAuditEntry(entryID);
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loadingDetail.value = false;
    }
  }

  function resetFilters() {
    querySources.value = [];
    queryKeyword.value = "";
    queryFrom.value = "";
    queryTo.value = "";
    queryTaskID.value = "";
    queryRunID.value = "";
    queryStreams.value = [...DEFAULT_STREAMS];
  }

  async function init() {
    error.value = "";
    try {
      await refreshMeta();
      await search();
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    }
  }

  return {
    loading,
    loadingMore,
    loadingDetail,
    loadingMeta,
    error,

    sources,
    retention,

    entries,
    nextCursor,
    selectedID,
    detail,

    querySources,
    queryKeyword,
    queryFrom,
    queryTo,
    queryTaskID,
    queryRunID,
    queryStreams,

    init,
    refreshMeta,
    search,
    loadMore,
    selectEntry,
    resetFilters,
  };
}


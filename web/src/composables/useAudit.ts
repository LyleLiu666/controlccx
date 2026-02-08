import { computed, ref } from "vue";
import {
  fetchAuditEntries,
  fetchAuditEntry,
  fetchAuditRetention,
  fetchAuditSources,
} from "../api";
import type {
  AuditEntry,
  AuditEntriesResponse,
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
const AUDIT_PAGE_SIZE = 50;

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
  const currentCursor = ref("");
  const previousCursors = ref<string[]>([]);
  const nextCursor = ref("");
  const pageNumber = ref(1);
  const selectedID = ref("");
  const detail = ref<AuditEntryDetail | null>(null);
  const hasPrevPage = computed(() => previousCursors.value.length > 0);
  const hasNextPage = computed(() => String(nextCursor.value ?? "").trim() !== "");

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
      limit: AUDIT_PAGE_SIZE,
      cursor: String(cursor ?? "").trim(),
    };
  }

  async function applyPage(res: AuditEntriesResponse | null | undefined, cursor: string) {
    entries.value = Array.isArray(res?.entries) ? res.entries : [];
    currentCursor.value = String(cursor ?? "").trim();
    nextCursor.value = String(res?.next_cursor ?? "");

    const firstID = entries.value[0]?.id ?? "";
    selectedID.value = firstID;
    if (firstID) {
      await selectEntry(firstID);
      return;
    }
    detail.value = null;
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
    if (loading.value || loadingMore.value) return;
    loading.value = true;
    error.value = "";
    try {
      const res = await fetchAuditEntries(buildQuery(""));
      previousCursors.value = [];
      pageNumber.value = 1;
      await applyPage(res, "");
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loading.value = false;
    }
  }

  async function loadNextPage() {
    const cursor = String(nextCursor.value ?? "").trim();
    if (!cursor || loading.value || loadingMore.value) return;
    loadingMore.value = true;
    error.value = "";
    const fromCursor = String(currentCursor.value ?? "").trim();
    try {
      const res = await fetchAuditEntries(buildQuery(cursor));
      previousCursors.value = [...previousCursors.value, fromCursor];
      pageNumber.value = previousCursors.value.length + 1;
      await applyPage(res, cursor);
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loadingMore.value = false;
    }
  }

  async function loadPrevPage() {
    if (loading.value || loadingMore.value) return;
    const trail = previousCursors.value.slice();
    if (trail.length === 0) return;
    const cursor = String(trail[trail.length - 1] ?? "").trim();
    loadingMore.value = true;
    error.value = "";
    try {
      const res = await fetchAuditEntries(buildQuery(cursor));
      previousCursors.value = trail.slice(0, -1);
      pageNumber.value = previousCursors.value.length + 1;
      await applyPage(res, cursor);
    } catch (e: any) {
      error.value = e?.message ?? String(e);
    } finally {
      loadingMore.value = false;
    }
  }

  async function loadMore() {
    await loadNextPage();
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
    currentCursor,
    previousCursors,
    nextCursor,
    pageNumber,
    selectedID,
    detail,
    hasPrevPage,
    hasNextPage,

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
    loadPrevPage,
    loadNextPage,
    loadMore,
    selectEntry,
    resetFilters,
  };
}

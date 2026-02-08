import { computed, onBeforeUnmount, ref, type Ref } from "vue";
import type { LogEntry, ServerEvent, Task, TaskTraceResponse } from "../types";
import { buildInstanceTokenHeaders, fetchLogs, fetchTaskTrace, fetchTasks, INSTANCE_TOKEN_REQUIRED_ERROR } from "../api";
import { deriveNextSelectedTaskId } from "../taskSelection";

export type UseTasksOptions = {
  showDeleted: Ref<boolean>;
  onTaskUpsert?: (prev: Task | undefined, next: Task) => void;
  autoSelectFirst?: boolean;
  onTokenRequired?: (message: string) => void;
};

export function useTasks(opts: UseTasksOptions) {
  const autoSelectFirst = opts.autoSelectFirst ?? true;
  const tasks = ref<Map<string, Task>>(new Map());
  const selectedTaskId = ref<string>("");
  const logsByTask = ref<Map<string, LogEntry[]>>(new Map());

  const traceByTask = ref<Map<string, TaskTraceResponse>>(new Map());
  const traceLoading = ref(false);
  const traceError = ref("");

  const eventsConnected = ref(true);
  const eventsLastEventMs = ref(Date.now());
  const eventsLastHeartbeatMs = ref(0);
  const eventsLastError = ref("");

  const selectedTask = computed(() => tasks.value.get(selectedTaskId.value) ?? null);
  const selectedLogs = computed(() => logsByTask.value.get(selectedTaskId.value) ?? []);
  const selectedTrace = computed(() => {
    const id = selectedTaskId.value;
    if (!id) return null;
    return traceByTask.value.get(id) ?? null;
  });

  function upsertTask(task: Task) {
    if (!opts.showDeleted.value && task.session_deleted_at) {
      const next = new Map(tasks.value);
      next.delete(task.id);
      tasks.value = next;
      if (selectedTaskId.value === task.id) {
        selectedTaskId.value = Array.from(next.keys())[0] ?? "";
      }
      return;
    }

    // Ensure reactivity for Map updates.
    const next = new Map(tasks.value);
    next.set(task.id, task);
    tasks.value = next;
    if (!selectedTaskId.value && autoSelectFirst) selectedTaskId.value = task.id;
  }

  function appendLog(entry: LogEntry) {
    const list = logsByTask.value.get(entry.task_id) ?? [];
    const next = new Map(logsByTask.value);
    next.set(entry.task_id, [...list, entry]);
    logsByTask.value = next;
  }

  async function refreshTasks(limit = 200) {
    const list = await fetchTasks(limit, opts.showDeleted.value);
    const next = new Map<string, Task>();
    for (const t of list) next.set(t.id, t);
    tasks.value = next;

    selectedTaskId.value = deriveNextSelectedTaskId({
      current: selectedTaskId.value,
      candidates: list,
      autoSelectFirst,
    });
    return list;
  }

  async function loadLogs(taskId: string) {
    const logs = await fetchLogs(taskId, 0, 500);
    const next = new Map(logsByTask.value);
    next.set(taskId, logs);
    logsByTask.value = next;
  }

  async function loadTrace(taskId: string) {
    if (!taskId) return;
    if (traceByTask.value.has(taskId)) return;
    traceError.value = "";
    traceLoading.value = true;
    try {
      const trace = await fetchTaskTrace(taskId);
      const next = new Map(traceByTask.value);
      next.set(taskId, trace);
      traceByTask.value = next;
    } catch (e: any) {
      traceError.value = e?.message ?? String(e);
    } finally {
      traceLoading.value = false;
    }
  }

  async function selectTask(taskId: string, opts?: { closeMobileDrawer?: () => void; closeRunsModal?: () => void }) {
    selectedTaskId.value = taskId;
    if (!logsByTask.value.has(taskId)) await loadLogs(taskId);
    opts?.closeMobileDrawer?.();
    opts?.closeRunsModal?.();
  }

  let eventsAbort: AbortController | null = null;
  let eventsLoopToken = 0;
  let normalizeOrderTimer: ReturnType<typeof setTimeout> | null = null;

  let lastEventSeq = 0;
  let resyncInFlight = false;
  let lastResyncMs = 0;

  const resyncThrottleMs = 1500;
  const periodicResyncMs = 60_000;
  const resyncLogsLimit = 2000;

  function mergeLogsByID(existing: LogEntry[], fetched: LogEntry[]) {
    if (existing.length === 0) return fetched;
    if (fetched.length === 0) return existing;
    const byID = new Map<number, LogEntry>();
    for (const e of existing) byID.set(e.id, e);
    for (const e of fetched) byID.set(e.id, e);
    return Array.from(byID.values()).sort((a, b) => a.id - b.id);
  }

  async function resync(reason: string, opts?: { fullLogs?: boolean }) {
    const now = Date.now();
    if (resyncInFlight) return;
    if (now - lastResyncMs < resyncThrottleMs) return;
    resyncInFlight = true;
    lastResyncMs = now;

    try {
      await refreshTasks(200);

      const taskID = selectedTaskId.value;
      if (!taskID) return;

      const existing = logsByTask.value.get(taskID) ?? [];
      const full = opts?.fullLogs || existing.length === 0;

      if (full) {
        const fetched = await fetchLogs(taskID, 0, resyncLogsLimit);
        const merged = mergeLogsByID(logsByTask.value.get(taskID) ?? [], fetched);
        const next = new Map(logsByTask.value);
        next.set(taskID, merged);
        logsByTask.value = next;
        return;
      }

      const lastID = existing[existing.length - 1]?.id ?? 0;
      const delta = await fetchLogs(taskID, lastID, resyncLogsLimit);
      if (delta.length === 0) return;
      const merged = mergeLogsByID(logsByTask.value.get(taskID) ?? [], delta);
      const next = new Map(logsByTask.value);
      next.set(taskID, merged);
      logsByTask.value = next;
    } catch {
      // ignore resync failures; SSE updates still keep task content fresh.
    } finally {
      resyncInFlight = false;
    }
  }

  function scheduleOrderNormalization() {
    if (normalizeOrderTimer) return;
    normalizeOrderTimer = setTimeout(async () => {
      normalizeOrderTimer = null;
      try {
        await refreshTasks(200);
      } catch {
        // ignore normalize failures; SSE updates still keep task content fresh.
      }
    }, 100);
  }

  function handleServerEvent(evt: ServerEvent) {
    eventsConnected.value = true;
    eventsLastEventMs.value = Date.now();

    const seq = Number(evt.seq ?? 0);
    if (seq > 0) {
      if (lastEventSeq > 0 && seq <= lastEventSeq) {
        void resync("events seq reset", { fullLogs: true });
      } else if (lastEventSeq > 0 && seq > lastEventSeq + 1) {
        void resync("events gap", { fullLogs: true });
      }
      lastEventSeq = seq;
    }

    if (evt.type === "heartbeat") {
      eventsLastHeartbeatMs.value = Date.now();
      eventsLastEventMs.value = eventsLastHeartbeatMs.value;
      if (Date.now() - lastResyncMs > periodicResyncMs) {
        void resync("events heartbeat");
      }
      return;
    }

    if (evt.type === "task.created" || evt.type === "task.updated") {
      const nextTask = evt.payload as Task;
      const prevTask = tasks.value.get(nextTask.id);
      upsertTask(nextTask);
      if (opts.onTaskUpsert) opts.onTaskUpsert(prevTask, nextTask);
      const orderKeyChanged =
        !prevTask ||
        prevTask.created_at !== nextTask.created_at ||
        prevTask.started_at !== nextTask.started_at ||
        prevTask.finished_at !== nextTask.finished_at;
      if (orderKeyChanged) scheduleOrderNormalization();
      return;
    }
    if (evt.type === "task.log") {
      appendLog(evt.payload as LogEntry);
    }
  }

  function parseSSEBlock(block: string): { eventName: string; dataRaw: string } | null {
    const lines = block.split(/\r?\n/);
    let eventName = "";
    const dataLines: string[] = [];
    for (const rawLine of lines) {
      const line = rawLine.trimEnd();
      if (!line) continue;
      if (line.startsWith("event:")) {
        eventName = line.slice("event:".length).trim();
        continue;
      }
      if (line.startsWith("data:")) {
        dataLines.push(line.slice("data:".length).trimStart());
      }
    }
    if (!eventName) return null;
    return { eventName, dataRaw: dataLines.join("\n") };
  }

  function isAbortError(e: any): boolean {
    const msg = String(e?.name || e?.message || "").toLowerCase();
    return msg.includes("abort");
  }

  function sleep(ms: number, signal?: AbortSignal): Promise<void> {
    return new Promise((resolve, reject) => {
      let settled = false;
      let t: ReturnType<typeof setTimeout> | null = null;

      function removeAbortListener() {
        if (!signal) return;
        try {
          signal.removeEventListener("abort", onAbort);
        } catch {
          // ignore
        }
      }

      function onAbort() {
        if (settled) return;
        settled = true;
        if (t) clearTimeout(t);
        removeAbortListener();
        reject(new Error("aborted"));
      }

      t = setTimeout(() => {
        if (settled) return;
        settled = true;
        removeAbortListener();
        resolve();
      }, ms);

      if (!signal) return;
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener("abort", onAbort);
    });
  }

  async function runEventsLoop(loopToken: number, signal: AbortSignal) {
    let attempt = 0;
    while (loopToken === eventsLoopToken && !signal.aborted) {
      try {
        const res = await fetch("/api/events", {
          method: "GET",
          headers: buildInstanceTokenHeaders({ "Accept": "text/event-stream" }),
          credentials: "same-origin",
          signal,
        });

        if (!res.ok) {
          let rawText = "";
          let err: any = null;
          try {
            rawText = await res.text();
            const ct = String(res.headers.get("Content-Type") || "").toLowerCase();
            if (ct.includes("application/json")) {
              try {
                err = JSON.parse(rawText);
              } catch {
                err = null;
              }
            }
          } catch {
            // ignore
          }

          const code = String(err?.error ?? "").trim();
          const msg = String(err?.message ?? rawText ?? res.statusText ?? "events connect failed").trim();
          eventsConnected.value = false;
          eventsLastError.value = msg || `HTTP ${res.status}`;

          if ((res.status === 401 || res.status === 403) && code === INSTANCE_TOKEN_REQUIRED_ERROR) {
            opts.onTokenRequired?.(msg || "missing instance token");
            return;
          }

          attempt += 1;
          await sleep(Math.min(10_000, 800 + attempt * 600), signal);
          continue;
        }

        const reader = res.body?.getReader();
        if (!reader) {
          eventsConnected.value = false;
          eventsLastError.value = "stream body unavailable";
          attempt += 1;
          await sleep(Math.min(10_000, 800 + attempt * 600), signal);
          continue;
        }

        // Connected.
        attempt = 0;
        eventsConnected.value = true;
        eventsLastError.value = "";
        eventsLastEventMs.value = Date.now();
        lastEventSeq = 0;
        void resync("events open", { fullLogs: true });

        const decoder = new TextDecoder();
        let pending = "";

        while (loopToken === eventsLoopToken && !signal.aborted) {
          const { done, value } = await reader.read();
          if (done) break;
          pending += decoder.decode(value, { stream: true });
          while (true) {
            const splitAt = pending.indexOf("\n\n");
            if (splitAt < 0) break;
            const block = pending.slice(0, splitAt);
            pending = pending.slice(splitAt + 2);
            const parsed = parseSSEBlock(block);
            if (!parsed) continue;
            if (!parsed.dataRaw) continue;
            try {
              const evt = JSON.parse(parsed.dataRaw) as ServerEvent;
              handleServerEvent(evt);
            } catch {
              // ignore
            }
          }
        }

        // Connection ended. Retry.
        eventsConnected.value = false;
        eventsLastError.value = "disconnected";
        attempt += 1;
        await sleep(Math.min(10_000, 800 + attempt * 600), signal);
      } catch (e: any) {
        if (signal.aborted || loopToken !== eventsLoopToken || isAbortError(e)) return;
        eventsConnected.value = false;
        eventsLastError.value = String(e?.message ?? e ?? "disconnected");
        attempt += 1;
        await sleep(Math.min(10_000, 800 + attempt * 600), signal);
      }
    }
  }

  function connectEvents() {
    // Force a new loop token so any in-flight reader exits.
    eventsLoopToken += 1;
    const token = eventsLoopToken;

    if (eventsAbort) {
      try {
        eventsAbort.abort();
      } catch {
        // ignore
      }
    }
    eventsAbort = new AbortController();

    eventsConnected.value = false;
    eventsLastError.value = "";
    eventsLastEventMs.value = Date.now();
    lastEventSeq = 0;

    void runEventsLoop(token, eventsAbort.signal);
  }

  function reconnectEvents() {
    connectEvents();
  }

  onBeforeUnmount(() => {
    if (normalizeOrderTimer) {
      clearTimeout(normalizeOrderTimer);
      normalizeOrderTimer = null;
    }
    eventsLoopToken += 1;
    if (!eventsAbort) return;
    try {
      eventsAbort.abort();
    } catch {
      // ignore
    }
    eventsAbort = null;
  });

  return {
    tasks,
    selectedTaskId,
    selectedTask,
    selectedLogs,
    logsByTask,
    traceByTask,
    traceLoading,
    traceError,
    selectedTrace,
    eventsConnected,
    eventsLastEventMs,
    eventsLastHeartbeatMs,
    eventsLastError,
    refreshTasks,
    loadLogs,
    loadTrace,
    selectTask,
    upsertTask,
    appendLog,
    connectEvents,
    reconnectEvents,
  };
}

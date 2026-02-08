import { computed, onBeforeUnmount, ref, type Ref } from "vue";
import type { LogEntry, ServerEvent, Task, TaskTraceResponse } from "../types";
import { fetchLogs, fetchTaskTrace, fetchTasks } from "../api";
import { deriveNextSelectedTaskId } from "../taskSelection";

export type UseTasksOptions = {
  showDeleted: Ref<boolean>;
  onTaskUpsert?: (prev: Task | undefined, next: Task) => void;
  autoSelectFirst?: boolean;
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

  let es: EventSource | null = null;
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

  function connectEvents() {
    if (es) {
      try {
        es.close();
      } catch {
        // ignore
      }
      es = null;
    }

    eventsConnected.value = true;
    eventsLastError.value = "";
    eventsLastEventMs.value = Date.now();
    lastEventSeq = 0;
    es = new EventSource("/api/events");

    es.onopen = () => {
      eventsConnected.value = true;
      eventsLastError.value = "";
      eventsLastEventMs.value = Date.now();
      void resync("events open", { fullLogs: true });
    };

    es.onerror = () => {
      // EventSource will auto-reconnect, but we surface status to the user.
      eventsConnected.value = false;
      eventsLastError.value = "disconnected";
    };

    const onAny = (e: MessageEvent) => {
      try {
        const evt = JSON.parse(e.data) as ServerEvent;
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
        } else if (evt.type === "task.log") {
          appendLog(evt.payload as LogEntry);
        }
      } catch {
        // ignore
      }
    };

    es.addEventListener("task.created", onAny);
    es.addEventListener("task.updated", onAny);
    es.addEventListener("task.log", onAny);
    es.addEventListener("hello", onAny);
    es.addEventListener("heartbeat", () => {
      eventsConnected.value = true;
      eventsLastHeartbeatMs.value = Date.now();
      eventsLastEventMs.value = eventsLastHeartbeatMs.value;
      if (Date.now() - lastResyncMs > periodicResyncMs) {
        void resync("events heartbeat");
      }
    });
  }

  function reconnectEvents() {
    connectEvents();
  }

  onBeforeUnmount(() => {
    if (normalizeOrderTimer) {
      clearTimeout(normalizeOrderTimer);
      normalizeOrderTimer = null;
    }
    if (!es) return;
    try {
      es.close();
    } catch {
      // ignore
    }
    es = null;
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

import { computed, onBeforeUnmount, ref, type Ref } from "vue";
import type { ChatMessage, LogEntry, ServerEvent, Task, TaskTraceResponse } from "../types";
import { fetchLogs, fetchTaskTrace, fetchTasks } from "../api";

export type UseTasksOptions = {
  showDeleted: Ref<boolean>;
  onTaskUpsert?: (prev: Task | undefined, next: Task) => void;
  onChatMessage?: (msg: ChatMessage) => void;
};

export function useTasks(opts: UseTasksOptions) {
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
    if (!selectedTaskId.value) selectedTaskId.value = task.id;
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

    if (selectedTaskId.value && !tasks.value.has(selectedTaskId.value)) {
      selectedTaskId.value = list[0]?.id ?? "";
    } else if (!selectedTaskId.value) {
      selectedTaskId.value = list[0]?.id ?? "";
    }
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
    es = new EventSource("/api/events");

    es.onopen = () => {
      eventsConnected.value = true;
      eventsLastError.value = "";
      eventsLastEventMs.value = Date.now();
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
        if (evt.type === "task.created" || evt.type === "task.updated") {
          const nextTask = evt.payload as Task;
          const prevTask = tasks.value.get(nextTask.id);
          upsertTask(nextTask);
          if (opts.onTaskUpsert) opts.onTaskUpsert(prevTask, nextTask);
        } else if (evt.type === "task.log") {
          appendLog(evt.payload as LogEntry);
        } else if (evt.type === "chat.message") {
          if (opts.onChatMessage) opts.onChatMessage(evt.payload as ChatMessage);
        }
      } catch {
        // ignore
      }
    };

    es.addEventListener("task.created", onAny);
    es.addEventListener("task.updated", onAny);
    es.addEventListener("task.log", onAny);
    es.addEventListener("chat.message", onAny);
    es.addEventListener("hello", onAny);
    es.addEventListener("heartbeat", () => {
      eventsConnected.value = true;
      eventsLastHeartbeatMs.value = Date.now();
      eventsLastEventMs.value = eventsLastHeartbeatMs.value;
    });
  }

  function reconnectEvents() {
    connectEvents();
  }

  onBeforeUnmount(() => {
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


import { computed, nextTick, ref, watch, type Ref } from "vue";
import type { LogEntry } from "../types";

export type FeedItem = {
  task_id: string;
  task_short: string;
  time: string;
  time_ms: number;
  stream: LogEntry["stream"];
  message: string;
};

function parseLogTimeMs(ts: string): number {
  const s = (ts ?? "").trim();
  const n = Date.parse(s);
  return Number.isFinite(n) ? n : 0;
}

function isMilestoneMessage(stream: LogEntry["stream"], message: string): boolean {
  const msg = (message ?? "").trim();
  if (!msg) return false;
  const lower = msg.toLowerCase();

  if (stream === "assistant") return true;

  if (stream === "system") {
    if (lower.startsWith("run.start")) return true;
    if (lower.startsWith("run.finish")) return true;
    if (lower.includes("blocked") || lower.includes("requires approval")) return true;
    if (lower.includes("error") || lower.includes("panic") || lower.includes("failed")) return true;
    if (lower.includes("skipped overlong") || lower.includes("read error")) return true;
    return false;
  }

  // stderr is noisy; keep only obvious problems.
  if (stream === "stderr") {
    if (lower.includes("error") || lower.includes("panic") || lower.includes("failed")) return true;
    return false;
  }

  // stdout is usually too chatty for milestones.
  return false;
}

function summarizeForFeed(stream: LogEntry["stream"], message: string): string {
  const msg = (message ?? "").trimEnd();
  if (!msg) return "";
  const max = stream === "assistant" ? 280 : 220;
  if (msg.length <= max) return msg;
  return msg.slice(0, max).trimEnd() + "…";
}

export function useLiveFeed(opts: {
  logsByTask: Ref<Map<string, LogEntry[]>>;
  eventsLastEventMs: Ref<number>;
  getCurrentRunIDs: () => string[];
  loadLogs: (taskId: string) => Promise<void>;
  onResizeEnd?: (width: number) => void;
}) {
  const liveOpen = ref(false);
  const liveScope = ref<"current" | "all">("current");
  const liveMode = ref<"milestones" | "all">("milestones");
  const livePaused = ref(false);
  const liveWrap = ref(true);
  const liveFull = ref(false);
  const liveWidth = ref(980);
  const liveResizing = ref(false);
  const liveBoxEl = ref<HTMLDivElement | null>(null);
  const liveNowMs = ref(Date.now());

  function startLiveResize(e: MouseEvent) {
    if (liveFull.value) return;
    liveResizing.value = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const startX = e.clientX;
    const startWidth = liveWidth.value;

    const onMove = (tm: MouseEvent) => {
      const diff = startX - tm.clientX;
      let newW = startWidth + diff;
      const maxW = Math.min(1600, window.innerWidth - 32);
      if (newW < 520) newW = 520;
      if (newW > maxW) newW = maxW;
      liveWidth.value = newW;
    };

    const onUp = () => {
      liveResizing.value = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      opts.onResizeEnd?.(liveWidth.value);
    };

    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }

  const liveItemsAll = computed<FeedItem[]>(() => {
    const scope = liveScope.value;
    const byTask: Array<{ taskId: string; logs: LogEntry[] }> = [];

    if (scope === "current") {
      for (const id of opts.getCurrentRunIDs()) {
        const logs = opts.logsByTask.value.get(id);
        if (!logs || logs.length === 0) continue;
        byTask.push({ taskId: id, logs });
      }
    } else {
      for (const [taskId, logs] of opts.logsByTask.value.entries()) {
        if (!logs || logs.length === 0) continue;
        byTask.push({ taskId, logs });
      }
    }

    const out: FeedItem[] = [];
    for (const { taskId, logs } of byTask) {
      for (const l of logs) {
        out.push({
          task_id: taskId,
          task_short: taskId.slice(0, 8),
          time: l.time,
          time_ms: parseLogTimeMs(l.time),
          stream: l.stream,
          message: l.message ?? "",
        });
      }
    }

    out.sort((a, b) => {
      const dm = a.time_ms - b.time_ms;
      if (dm !== 0) return dm;
      return a.time.localeCompare(b.time);
    });

    const max = 240;
    return out.length > max ? out.slice(out.length - max) : out;
  });

  const liveItems = computed<FeedItem[]>(() => {
    if (liveMode.value === "all") return liveItemsAll.value;
    const out = liveItemsAll.value.filter((f) => isMilestoneMessage(f.stream, f.message));
    return out.map((f) => ({ ...f, message: summarizeForFeed(f.stream, f.message) }));
  });

  const liveLastTimeMsAll = computed(() => {
    const list = liveItemsAll.value;
    if (list.length === 0) return 0;
    return list[list.length - 1].time_ms;
  });

  const eventsIdleSeconds = computed(() => {
    const last = opts.eventsLastEventMs.value;
    if (!last) return 0;
    const s = Math.floor((liveNowMs.value - last) / 1000);
    return s > 0 ? s : 0;
  });

  const feedIdleSeconds = computed(() => {
    const last = liveLastTimeMsAll.value;
    if (!last) return 0;
    const s = Math.floor((liveNowMs.value - last) / 1000);
    return s > 0 ? s : 0;
  });

  watch(
    [liveOpen, liveScope],
    async ([open]) => {
      if (!open) return;
      await nextTick();
      // Backfill a small amount of logs to avoid "blank" Live after refresh.
      if (liveScope.value === "current") {
        const runs = opts.getCurrentRunIDs().slice(-6);
        await Promise.all(
          runs.map(async (id) => {
            const existing = opts.logsByTask.value.get(id);
            if (existing && existing.length > 0) return;
            try {
              await opts.loadLogs(id);
            } catch {
              // ignore
            }
          }),
        );
      }
      if (!livePaused.value) {
        const el = liveBoxEl.value;
        if (el) el.scrollTop = el.scrollHeight;
      }
    },
    { immediate: false },
  );

  watch(
    liveOpen,
    (open) => {
      if (!open) return;
      // Avoid evaluating liveItems before the drawer is opened (keeps the composable
      // safe to initialize before "current run" context is ready).
      const stopItemsWatch = watch(
        () => liveItems.value.length,
        async () => {
          if (!liveOpen.value) return;
          if (livePaused.value) return;
          await nextTick();
          const el = liveBoxEl.value;
          if (el) el.scrollTop = el.scrollHeight;
        },
        { immediate: false },
      );
      const stopOpenWatch = watch(
        liveOpen,
        (stillOpen) => {
          if (stillOpen) return;
          stopItemsWatch();
          stopOpenWatch();
        },
        { immediate: false },
      );
    },
    { immediate: true },
  );

  let liveTimer: number | null = null;
  watch(
    [liveOpen],
    ([open]) => {
      if (liveTimer != null) {
        window.clearInterval(liveTimer);
        liveTimer = null;
      }
      if (!open) return;
      liveNowMs.value = Date.now();
      liveTimer = window.setInterval(() => {
        liveNowMs.value = Date.now();
      }, 1000);
    },
    { immediate: true },
  );

  return {
    liveOpen,
    liveScope,
    liveMode,
    livePaused,
    liveWrap,
    liveFull,
    liveWidth,
    liveResizing,
    liveBoxEl,
    eventsIdleSeconds,
    feedIdleSeconds,
    liveItems,
    startLiveResize,
  };
}

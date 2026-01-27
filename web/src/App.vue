<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type {
  AuthInfo,
  AuthPatch,
  AuthStatus,
  ChatMessage,
  FSListEntry,
  FSRoot,
  LogEntry,
  ServerEvent,
  SystemInfo,
  Task,
  WorkerType,
} from "./types";
import {
  cancelTask,
  createTask,
  fetchAuthInfo,
  fetchChat,
  fetchFSList,
  fetchFSRoots,
  fetchLogs,
  fetchSystemInfo,
  fetchTasks,
  resumeTask,
  sendChat,
  sendChatStream,
  updateAuth,
} from "./api";
import { appendChatMessageUnique, sendChatAndReload } from "./chatOps";

const tasks = ref<Map<string, Task>>(new Map());
const selectedTaskId = ref<string>("");
const logsByTask = ref<Map<string, LogEntry[]>>(new Map());

const systemInfo = ref<SystemInfo | null>(null);
const chat = ref<ChatMessage[]>([]);

const newWorkerType = ref<WorkerType>("claude-code");
const newWorkdir = ref<string>(".");
const newPrompt = ref<string>("");
const newRunOpen = ref(false);
const newRunPromptEl = ref<HTMLTextAreaElement | null>(null);

const resumePrompt = ref<string>("");
const resumeExpanded = ref(false);
const chatInput = ref<string>("");
const chatInputEl = ref<HTMLTextAreaElement | null>(null);
const errorBanner = ref<string>("");
const chatBackend = ref<"auto" | "claude" | "codex">("auto");
const chatStreamEnabled = ref<boolean>(true);
const chatMaxSteps = ref<number>(8);
const chatStreamStatus = ref<string>("");
const chatStreamAnswer = ref<string>("");
const chatSending = ref<boolean>(false);

const theme = ref<"light" | "dark">("light");

const authInfo = ref<AuthInfo | null>(null);
const authStatus = computed<AuthStatus | null>(
  () => authInfo.value?.status ?? null,
);
const authSettingsOpen = ref(false);
const authSaving = ref(false);
const authSettingsError = ref("");
const authAnthropicBaseURL = ref("");
const authAnthropicApiKey = ref("");
const authAnthropicAuthToken = ref("");
const authAnthropicModel = ref("");
const authAnthropicSmallFastModel = ref("");
const authOpenAIApiKey = ref("");
const authCodexModel = ref("");
const authCodexReasoningEffort = ref("");

const selectedTask = computed(
  () => tasks.value.get(selectedTaskId.value) ?? null,
);
const selectedLogs = computed(
  () => logsByTask.value.get(selectedTaskId.value) ?? [],
);

const outputTab = ref<"result" | "logs">("result");
const logShowAssistant = ref(true);
const logShowStdout = ref(true);
const logShowStderr = ref(true);
const logShowSystem = ref(true);
const logSearch = ref("");
const sessionSearch = ref("");

const secretaryOpen = ref(false);
const secretaryView = ref<"chat" | "overview" | "feed">("chat");
const feedScope = ref<"current" | "all">("current");
const feedPaused = ref(false);
const feedWrap = ref(true);
const feedBoxEl = ref<HTMLDivElement | null>(null);
const feedNowMs = ref(Date.now());

function formatLogTime(ts: string): string {
  const s = ts.trim();
  if (s.length >= 19) return s.slice(11, 19);
  return s;
}

const selectedAssistantResult = computed(() => {
  let best = "";
  for (const l of selectedLogs.value) {
    if (l.stream !== "assistant") continue;
    const msg = l.message ?? "";
    if (msg.length > best.length) best = msg;
  }
  return best.trim();
});

const selectedStdoutText = computed(() => {
  return selectedLogs.value
    .filter((l) => l.stream === "stdout" && l.message)
    .map((l) => l.message)
    .join("\n");
});

const selectedStderrText = computed(() => {
  return selectedLogs.value
    .filter((l) => l.stream === "stderr" && l.message)
    .map((l) => l.message)
    .join("\n");
});

const selectedResultText = computed(() => {
  if (selectedAssistantResult.value) return selectedAssistantResult.value;
  const out = selectedStdoutText.value.trim();
  const err = selectedStderrText.value.trim();
  if (out && err) return `${out}\n\n[stderr]\n${err}`;
  return out || err;
});

const filteredLogs = computed(() => {
  const q = logSearch.value.trim().toLowerCase();
  return selectedLogs.value.filter((l) => {
    if (l.stream === "assistant" && !logShowAssistant.value) return false;
    if (l.stream === "stdout" && !logShowStdout.value) return false;
    if (l.stream === "stderr" && !logShowStderr.value) return false;
    if (l.stream === "system" && !logShowSystem.value) return false;
    if (!q) return true;
    return (
      l.stream.toLowerCase().includes(q) ||
      (l.message ?? "").toLowerCase().includes(q)
    );
  });
});

async function copySelectedResult() {
  const text = selectedResultText.value;
  if (!text) return;
  await copyText(text);
}

async function copyText(text: string) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // Fallback for older browsers.
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
      document.execCommand("copy");
    } finally {
      document.body.removeChild(ta);
    }
  }
}

const dirPickerOpen = ref(false);
const dirRoots = ref<FSRoot[]>([]);
const dirPath = ref<string>("");
const dirParent = ref<string>("");
const dirEntries = ref<FSListEntry[]>([]);
const dirLoading = ref(false);
const dirFilter = ref("");
const dirError = ref("");

const filteredDirEntries = computed(() => {
  const needle = dirFilter.value.trim().toLowerCase();
  if (!needle) return dirEntries.value;
  return dirEntries.value.filter((e) => e.name.toLowerCase().includes(needle));
});

const LS_KEY_PINNED_WORKSPACES = "controlccx.pinned_workspaces.v1";
const LS_KEY_WORKSPACE_FILTER = "controlccx.workspace_filter.v1";
const LS_KEY_CHAT_BACKEND = "controlccx.chat.backend.v1";
const LS_KEY_CHAT_STREAM = "controlccx.chat.stream.v1";
const LS_KEY_CHAT_MAX_STEPS = "controlccx.chat.max_steps.v1";
const LS_KEY_SECRETARY_VIEW = "controlccx.secretary.view.v1";
const LS_KEY_THEME = "controlccx.theme.v1";
const LS_KEY_FEED_SCOPE = "controlccx.feed.scope.v1";
const LS_KEY_FEED_WRAP = "controlccx.feed.wrap.v1";

function getLocalStorage(): Storage | null {
  try {
    return window?.localStorage ?? null;
  } catch {
    return null;
  }
}

function loadStringArray(key: string): string[] {
  const st = getLocalStorage();
  if (!st) return [];
  try {
    const raw = st.getItem(key);
    if (!raw) return [];
    const v = JSON.parse(raw);
    if (!Array.isArray(v)) return [];
    return v
      .map((x) => (typeof x === "string" ? x.trim() : ""))
      .filter(Boolean);
  } catch {
    return [];
  }
}

function saveStringArray(key: string, items: string[]) {
  const st = getLocalStorage();
  if (!st) return;
  try {
    st.setItem(key, JSON.stringify(items));
  } catch {
    // ignore
  }
}

function loadString(key: string): string {
  const st = getLocalStorage();
  if (!st) return "";
  try {
    return st.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function saveString(key: string, value: string) {
  const st = getLocalStorage();
  if (!st) return;
  try {
    if (value.trim()) st.setItem(key, value);
    else st.removeItem(key);
  } catch {
    // ignore
  }
}

function loadBool(key: string, def: boolean): boolean {
  const raw = loadString(key).trim().toLowerCase();
  if (raw === "1" || raw === "true" || raw === "yes") return true;
  if (raw === "0" || raw === "false" || raw === "no") return false;
  return def;
}

function saveBool(key: string, value: boolean) {
  saveString(key, value ? "1" : "0");
}

function loadInt(key: string, def: number): number {
  const raw = loadString(key).trim();
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) ? n : def;
}

function saveInt(key: string, value: number) {
  if (!Number.isFinite(value)) return;
  saveString(key, String(value));
}

function applyTheme(t: "light" | "dark") {
  theme.value = t;
  try {
    document.documentElement.dataset.theme = t;
  } catch {
    // ignore
  }
}

function parseLogTimeMs(ts: string): number {
  const s = (ts ?? "").trim();
  const n = Date.parse(s);
  return Number.isFinite(n) ? n : 0;
}

function normalizePathForCompare(p: string): string {
  let s = p.trim();
  if (!s) return "";
  s = s.replaceAll("\\", "/").replace(/\/+/g, "/");
  while (s.length > 1 && s.endsWith("/")) s = s.slice(0, -1);
  if (/^[a-zA-Z]:/.test(s)) s = s.toLowerCase();
  return s;
}

function isWithinWorkspace(root: string, path: string): boolean {
  const r = normalizePathForCompare(root);
  if (!r) return true;
  if (r === "/") return true;
  const p = normalizePathForCompare(path);
  if (!p) return false;
  return p === r || p.startsWith(r + "/");
}

const pinnedWorkspaces = ref<string[]>(
  loadStringArray(LS_KEY_PINNED_WORKSPACES),
);
const workspaceFilter = ref<string>(loadString(LS_KEY_WORKSPACE_FILTER));

{
  const v = loadString(LS_KEY_CHAT_BACKEND).trim();
  if (v === "auto" || v === "claude" || v === "codex")
    chatBackend.value = v as any;
  chatStreamEnabled.value = loadBool(LS_KEY_CHAT_STREAM, true);
  const n = loadInt(LS_KEY_CHAT_MAX_STEPS, 8);
  chatMaxSteps.value = Math.max(1, Math.min(32, n));

  const sec = loadString(LS_KEY_SECRETARY_VIEW).trim();
  if (sec === "chat" || sec === "overview" || sec === "feed")
    secretaryView.value = sec;

  const fs = loadString(LS_KEY_FEED_SCOPE).trim();
  if (fs === "current" || fs === "all") feedScope.value = fs;
  feedWrap.value = loadBool(LS_KEY_FEED_WRAP, true);

  const t = loadString(LS_KEY_THEME).trim();
  if (t === "dark" || t === "light") {
    applyTheme(t);
  } else {
    const prefersDark =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;
    applyTheme(prefersDark ? "dark" : "light");
  }
}

watch(pinnedWorkspaces, (v) => saveStringArray(LS_KEY_PINNED_WORKSPACES, v), {
  deep: true,
});
watch(workspaceFilter, (v) => {
  saveString(LS_KEY_WORKSPACE_FILTER, v);
  if (v.trim()) newWorkdir.value = v;
});
watch(chatBackend, (v) => saveString(LS_KEY_CHAT_BACKEND, v));
watch(chatStreamEnabled, (v) => saveBool(LS_KEY_CHAT_STREAM, v));
watch(chatMaxSteps, (v) => saveInt(LS_KEY_CHAT_MAX_STEPS, v));
watch(secretaryView, (v) => saveString(LS_KEY_SECRETARY_VIEW, v));
watch(theme, (v) => saveString(LS_KEY_THEME, v));
watch(feedScope, (v) => saveString(LS_KEY_FEED_SCOPE, v));
watch(feedWrap, (v) => saveBool(LS_KEY_FEED_WRAP, v));

watch(selectedTaskId, () => {
  const t = selectedTask.value;
  if (!t) return;
  const isLLM = t.worker_type === "claude-code" || t.worker_type === "codex";
  outputTab.value = isLLM ? "result" : "logs";
  logShowAssistant.value = true;
  logShowStdout.value = true;
  logShowStderr.value = true;
  logShowSystem.value = true;
  logSearch.value = "";
  resumeExpanded.value = false;
});

type SessionGroup = {
  key: string;
  session_id: string;
  worker_type: WorkerType;
  workdir: string;
  status: Task["status"];
  score: number;
  stderr_count: number;
  warning: string;
  updated_at: string;
  latest: Task;
  runs: Task[];
};

function sessionKeyForTask(t: Task): string {
  const sid = t.session_id?.trim();
  if (sid) return `s:${sid}`;
  return `t:${t.id}`;
}

let es: EventSource | null = null;

function upsertTask(task: Task) {
  // Ensure reactivity for Map updates (some environments don't track Map mutations reliably).
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

async function refresh() {
  const [sys, taskList, chatList] = await Promise.all([
    fetchSystemInfo(),
    fetchTasks(),
    fetchChat(),
  ]);
  systemInfo.value = sys;
  taskList.forEach((t) => upsertTask(t));
  chat.value = chatList;
}

async function refreshAuth() {
  try {
    authInfo.value = await fetchAuthInfo();
  } catch {
    // ignore auth status failures (UI still works; tasks will surface logs)
  }
}

async function loadLogs(taskId: string) {
  const logs = await fetchLogs(taskId, 0, 500);
  const next = new Map(logsByTask.value);
  next.set(taskId, logs);
  logsByTask.value = next;
}

async function onCreateTask(): Promise<boolean> {
  errorBanner.value = "";
  try {
    const t = await createTask({
      worker_type: newWorkerType.value,
      prompt: newPrompt.value,
      workdir: newWorkdir.value,
    });
    upsertTask(t);
    selectedTaskId.value = t.id;
    newPrompt.value = "";
    await loadLogs(t.id);
    return true;
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
    return false;
  }
}

async function onSelectTask(id: string) {
  selectedTaskId.value = id;
  if (!logsByTask.value.has(id)) await loadLogs(id);
}

function openNewRun() {
  newRunOpen.value = true;
}

function closeNewRun() {
  newRunOpen.value = false;
}

async function onCreateTaskFromModal() {
  if (!newPrompt.value.trim()) return;
  if (!newWorkdir.value.trim()) return;
  if (missingAuthText.value) return;
  const ok = await onCreateTask();
  if (ok) closeNewRun();
}

function toggleSecretary() {
  secretaryOpen.value = !secretaryOpen.value;
}

function closeSecretary() {
  secretaryOpen.value = false;
}

function toggleTheme() {
  applyTheme(theme.value === "dark" ? "light" : "dark");
}

async function onCancelTask() {
  if (!selectedTaskId.value) return;
  errorBanner.value = "";
  try {
    await cancelTask(selectedTaskId.value);
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

async function onResumeTask() {
  const sess = selectedSession.value;
  if (!sess) return;
  if (!sess.session_id) {
    errorBanner.value = "该 session 还没有 session_id，无法 resume。";
    return;
  }
  errorBanner.value = "";
  try {
    const nt = await resumeTask(sess.latest.id, resumePrompt.value);
    upsertTask(nt);
    selectedTaskId.value = nt.id;
    resumePrompt.value = "";
    await loadLogs(nt.id);
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

async function onSendChat() {
  const msg = chatInput.value.trim();
  if (!msg) return;
  if (chatSending.value) return;
  errorBanner.value = "";
  chatSending.value = true;
  try {
    if (!chatStreamEnabled.value) {
      chat.value = await sendChatAndReload(msg, { sendChat, fetchChat });
      chatInput.value = "";
      return;
    }

    chatStreamStatus.value = "thinking";
    chatStreamAnswer.value = "";
    chatInput.value = "";

    await sendChatStream(
      msg,
      { backend: chatBackend.value, max_steps: chatMaxSteps.value },
      (evt) => {
        if (evt.event === "status") {
          const phase = String(evt.data?.phase ?? "");
          if (phase) chatStreamStatus.value = phase;
          return;
        }
        if (evt.event === "tool_call") {
          const tool = String(evt.data?.tool ?? "");
          if (tool) chatStreamStatus.value = `tool: ${tool}`;
          return;
        }
        if (evt.event === "tool_result") {
          const tool = String(evt.data?.tool ?? "");
          if (tool) chatStreamStatus.value = `tool done: ${tool}`;
          return;
        }
        if (evt.event === "final") {
          const m = String(evt.data?.message ?? "");
          if (m) chatStreamAnswer.value = m;
          chatStreamStatus.value = "";
        }
      },
    );

    chat.value = await fetchChat();
    chatStreamStatus.value = "";
    chatStreamAnswer.value = "";
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  } finally {
    chatSending.value = false;
  }
}

async function openDirPicker() {
  dirPickerOpen.value = true;
  dirError.value = "";
  dirFilter.value = "";
  try {
    dirRoots.value = await fetchFSRoots();
  } catch (e: any) {
    dirError.value = e?.message ?? String(e);
    dirRoots.value = [];
  }

  const initial = newWorkdir.value || (dirRoots.value[0]?.path ?? ".");
  await loadDir(initial);
}

async function loadDir(path: string) {
  dirLoading.value = true;
  dirError.value = "";
  try {
    const res = await fetchFSList(path);
    dirPath.value = res.path;
    dirParent.value = res.parent ?? "";
    dirEntries.value = res.entries;
  } catch (e: any) {
    dirError.value = e?.message ?? String(e);
  } finally {
    dirLoading.value = false;
  }
}

function selectDir(path: string) {
  newWorkdir.value = path;
  dirPickerOpen.value = false;
}

function setWorkspace(path: string) {
  workspaceFilter.value = path;
}

function clearWorkspace() {
  workspaceFilter.value = "";
}

function pinWorkspace(path: string) {
  const p = path.trim();
  if (!p) return;
  const key = normalizePathForCompare(p);
  const existing = pinnedWorkspaces.value.filter(Boolean);
  if (existing.some((x) => normalizePathForCompare(x) === key)) return;
  pinnedWorkspaces.value = [p, ...existing].slice(0, 12);
}

function unpinWorkspace(path: string) {
  const key = normalizePathForCompare(path);
  pinnedWorkspaces.value = pinnedWorkspaces.value.filter(
    (x) => normalizePathForCompare(x) !== key,
  );
  if (normalizePathForCompare(workspaceFilter.value) === key)
    workspaceFilter.value = "";
}

function connectEvents() {
  es = new EventSource("/api/events");

  const onAny = (e: MessageEvent) => {
    try {
      const evt = JSON.parse(e.data) as ServerEvent;
      if (evt.type === "task.created" || evt.type === "task.updated") {
        upsertTask(evt.payload as Task);
      } else if (evt.type === "task.log") {
        appendLog(evt.payload as LogEntry);
      } else if (evt.type === "chat.message") {
        chat.value = appendChatMessageUnique(chat.value, evt.payload as ChatMessage);
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
  es.addEventListener("heartbeat", () => {});
}

function openAuthSettings() {
  authSettingsError.value = "";
  authSettingsOpen.value = true;
  refreshAuth();
}

async function saveAuthSettings() {
  authSettingsError.value = "";
  authSaving.value = true;
  try {
    const patch: AuthPatch = {};
    if (authAnthropicBaseURL.value.trim())
      patch.anthropic_base_url = authAnthropicBaseURL.value.trim();
    if (authAnthropicApiKey.value.trim())
      patch.anthropic_api_key = authAnthropicApiKey.value.trim();
    if (authAnthropicAuthToken.value.trim())
      patch.anthropic_auth_token = authAnthropicAuthToken.value.trim();
    if (authAnthropicModel.value.trim())
      patch.anthropic_model = authAnthropicModel.value.trim();
    if (authAnthropicSmallFastModel.value.trim())
      patch.anthropic_small_fast_model =
        authAnthropicSmallFastModel.value.trim();
    if (authOpenAIApiKey.value.trim())
      patch.openai_api_key = authOpenAIApiKey.value.trim();
    if (authCodexModel.value.trim())
      patch.codex_model = authCodexModel.value.trim();
    if (authCodexReasoningEffort.value.trim())
      patch.codex_reasoning_effort = authCodexReasoningEffort.value.trim();

    if (Object.keys(patch).length > 0) {
      authInfo.value = await updateAuth(patch);
    } else {
      await refreshAuth();
    }

    authAnthropicBaseURL.value = "";
    authAnthropicApiKey.value = "";
    authAnthropicAuthToken.value = "";
    authAnthropicModel.value = "";
    authAnthropicSmallFastModel.value = "";
    authOpenAIApiKey.value = "";
    authCodexModel.value = "";
    authCodexReasoningEffort.value = "";
  } catch (e: any) {
    authSettingsError.value = e?.message ?? String(e);
  } finally {
    authSaving.value = false;
  }
}

async function clearStoredAuth(field: keyof AuthPatch) {
  authSettingsError.value = "";
  authSaving.value = true;
  try {
    authInfo.value = await updateAuth({ [field]: "" } as AuthPatch);
  } catch (e: any) {
    authSettingsError.value = e?.message ?? String(e);
  } finally {
    authSaving.value = false;
  }
}

const missingAuthText = computed(() => {
  const st = authStatus.value;
  if (!st) return "";
  if (newWorkerType.value === "claude-code" && !st.claude.available) {
    return "claude-code 未检测到可用鉴权：请设置 ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN，或在终端运行一次 `claude /login`。";
  }
  if (newWorkerType.value === "codex" && !st.codex.available) {
    return "codex 未检测到可用鉴权：请设置 OPENAI_API_KEY。";
  }
  return "";
});

onMounted(async () => {
  await refresh();
  if (selectedTaskId.value) await loadLogs(selectedTaskId.value);
  await refreshAuth();
  connectEvents();
  window.addEventListener("keydown", onGlobalKeyDown);
});

onBeforeUnmount(() => {
  if (es) es.close();
  window.removeEventListener("keydown", onGlobalKeyDown);
});

const sortedTasks = computed(() => {
  return Array.from(tasks.value.values()).sort((a, b) => {
    if (a.score === b.score) return b.created_at.localeCompare(a.created_at);
    return b.score - a.score;
  });
});

const selectedSessionKey = computed(() => {
  const t = selectedTask.value;
  if (!t) return "";
  return sessionKeyForTask(t);
});

const sessionsAll = computed<SessionGroup[]>(() => {
  const groups = new Map<string, Task[]>();
  for (const t of tasks.value.values()) {
    const key = sessionKeyForTask(t);
    const list = groups.get(key) ?? [];
    list.push(t);
    groups.set(key, list);
  }

  const out: SessionGroup[] = [];
  for (const [key, runs] of groups.entries()) {
    runs.sort((a, b) => a.created_at.localeCompare(b.created_at));
    const latest = runs[runs.length - 1];

    let score = 0;
    let stderrCount = 0;
    let warning = "";
    for (const r of runs) {
      score = Math.max(score, r.score);
      stderrCount = Math.max(stderrCount, r.stderr_count);
      if (!warning && r.warning) warning = r.warning;
    }

    out.push({
      key,
      session_id: latest.session_id?.trim() ?? "",
      worker_type: latest.worker_type,
      workdir: latest.workdir,
      status: latest.status,
      score,
      stderr_count: stderrCount,
      warning,
      updated_at: latest.updated_at,
      latest,
      runs,
    });
  }

  out.sort((a, b) => {
    if (a.score === b.score) return b.updated_at.localeCompare(a.updated_at);
    return b.score - a.score;
  });
  return out;
});

const filteredSessions = computed(() => {
  const root = workspaceFilter.value.trim();
  const needle = sessionSearch.value.trim().toLowerCase();
  return sessionsAll.value.filter((s) => {
    if (root && !isWithinWorkspace(root, s.workdir)) return false;
    if (!needle) return true;

    const sid = (s.session_id || s.latest.id).toLowerCase();
    const prompt = (s.latest.prompt ?? "").toLowerCase();
    const workdir = (s.workdir ?? "").toLowerCase();
    return (
      sid.includes(needle) ||
      prompt.includes(needle) ||
      workdir.includes(needle) ||
      s.worker_type.toLowerCase().includes(needle) ||
      s.status.toLowerCase().includes(needle)
    );
  });
});

const selectedSession = computed(() => {
  const key = selectedSessionKey.value;
  if (!key) return null;
  return sessionsAll.value.find((s) => s.key === key) ?? null;
});

const recentWorkspaces = computed(() => {
  const latestByPath = new Map<string, string>();
  for (const t of tasks.value.values()) {
    const p = t.workdir?.trim();
    if (!p) continue;
    const prev = latestByPath.get(p);
    if (!prev || t.created_at > prev) latestByPath.set(p, t.created_at);
  }
  return Array.from(latestByPath.entries())
    .sort((a, b) => b[1].localeCompare(a[1]))
    .map(([p]) => p)
    .slice(0, 20);
});

const recentWorkspacesUnpinned = computed(() => {
  const pinned = new Set(
    pinnedWorkspaces.value.map((p) => normalizePathForCompare(p)),
  );
  return recentWorkspaces.value.filter(
    (p) => !pinned.has(normalizePathForCompare(p)),
  );
});

const secretaryCounts = computed(() => {
  const out: Record<string, number> = {
    total: sessionsAll.value.length,
    running: 0,
    queued: 0,
    blocked: 0,
    failed: 0,
    interrupted: 0,
    succeeded: 0,
    canceled: 0,
  };
  for (const s of sessionsAll.value) {
    out[s.status] = (out[s.status] ?? 0) + 1;
  }
  return out;
});

const needsAttentionSessions = computed(() => {
  return sessionsAll.value
    .filter(
      (s) =>
        s.status !== "succeeded" &&
        (s.score > 0 || s.status === "failed" || s.status === "blocked"),
    )
    .slice(0, 6);
});

const secretaryBriefing = computed(() => {
  const c = secretaryCounts.value;
  if (c.total === 0) return "当前还没有 session。";

  const lines: string[] = [];
  lines.push(`Session 总数：${c.total}`);
  lines.push(
    `running ${c.running} · blocked ${c.blocked} · failed ${c.failed} · interrupted ${c.interrupted} · queued ${c.queued} · succeeded ${c.succeeded}`,
  );

  const top = needsAttentionSessions.value;
  if (top.length === 0) {
    lines.push("");
    lines.push("需要关注：暂无（看起来都很顺利）。");
    return lines.join("\n");
  }

  lines.push("");
  lines.push("需要关注（按 score / 最近更新）：");
  for (const s of top) {
    const sid = s.session_id
      ? s.session_id.slice(0, 8)
      : s.latest.id.slice(0, 8);
    lines.push(`- ${sid} · ${s.status} · score ${s.score} · ${s.workdir}`);
  }
  return lines.join("\n");
});

type FeedItem = {
  task_id: string;
  task_short: string;
  time: string;
  time_ms: number;
  stream: LogEntry["stream"];
  message: string;
};

const feedItems = computed<FeedItem[]>(() => {
  const scope = feedScope.value;
  const byTask: Array<{ taskId: string; logs: LogEntry[] }> = [];

  if (scope === "current") {
    const sess = selectedSession.value;
    if (!sess) return [];
    for (const r of sess.runs) {
      const logs = logsByTask.value.get(r.id);
      if (!logs || logs.length === 0) continue;
      byTask.push({ taskId: r.id, logs });
    }
  } else {
    for (const [taskId, logs] of logsByTask.value.entries()) {
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

const feedLastTimeMs = computed(() => {
  const list = feedItems.value;
  if (list.length === 0) return 0;
  return list[list.length - 1].time_ms;
});

const feedIdleSeconds = computed(() => {
  const last = feedLastTimeMs.value;
  if (!last) return 0;
  const s = Math.floor((feedNowMs.value - last) / 1000);
  return s > 0 ? s : 0;
});

function shouldIgnoreGlobalHotkey(e: KeyboardEvent): boolean {
  const t = e.target as any;
  if (!t) return false;
  if (t.isContentEditable) return true;
  const tag = String(t.tagName ?? "").toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

function onGlobalKeyDown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    if (dirPickerOpen.value) {
      dirPickerOpen.value = false;
      return;
    }
    if (authSettingsOpen.value) {
      authSettingsOpen.value = false;
      return;
    }
    if (newRunOpen.value) {
      closeNewRun();
      return;
    }
    if (secretaryOpen.value) {
      closeSecretary();
      return;
    }
    return;
  }

  if (shouldIgnoreGlobalHotkey(e)) return;
  if (e.ctrlKey || e.metaKey || e.altKey) return;

  if (e.key === "n" || e.key === "N") {
    e.preventDefault();
    openNewRun();
  }
  if (e.key === "s" || e.key === "S") {
    e.preventDefault();
    toggleSecretary();
  }
}

watch(newRunOpen, async (open) => {
  if (!open) return;
  await nextTick();
  newRunPromptEl.value?.focus();
});

watch(
  [secretaryOpen, secretaryView],
  async ([open, view]) => {
    if (!open) return;
    if (view !== "chat") return;
    await nextTick();
    chatInputEl.value?.focus();
  },
  { immediate: false },
);

watch(
  [secretaryOpen, secretaryView, feedScope],
  async ([open, view]) => {
    if (!open) return;
    if (view !== "feed") return;
    await nextTick();
    if (!feedPaused.value) {
      const el = feedBoxEl.value;
      if (el) el.scrollTop = el.scrollHeight;
    }
  },
  { immediate: false },
);

watch(
  () => feedItems.value.length,
  async () => {
    if (!secretaryOpen.value) return;
    if (secretaryView.value !== "feed") return;
    if (feedPaused.value) return;
    await nextTick();
    const el = feedBoxEl.value;
    if (el) el.scrollTop = el.scrollHeight;
  },
);

let feedTimer: number | null = null;
watch(
  [secretaryOpen, secretaryView],
  ([open, view]) => {
    if (feedTimer != null) {
      window.clearInterval(feedTimer);
      feedTimer = null;
    }
    if (!open || view !== "feed") return;
    feedNowMs.value = Date.now();
    feedTimer = window.setInterval(() => {
      feedNowMs.value = Date.now();
    }, 1000);
  },
  { immediate: true },
);
</script>

<template>
  <div class="page">
    <header class="header">
      <div class="title">ControlCCX</div>
      <div class="headerRight">
        <div class="sub" v-if="systemInfo">
          {{ systemInfo.os }}/{{ systemInfo.arch }} ·
          {{ systemInfo.hostname }} · Go {{ systemInfo.go_version }}
        </div>
        <button type="button" class="themeBtn" @click="toggleTheme">
          {{ theme === "dark" ? "Day" : "Night" }}
        </button>
        <button type="button" class="primary" @click="openNewRun">
          New Run
        </button>
        <button type="button" class="settingsBtn" @click="openAuthSettings">
          Settings
        </button>
      </div>
    </header>

    <div v-if="errorBanner" class="banner">{{ errorBanner }}</div>

    <div class="grid">
      <section class="panel">
        <h2>Sessions</h2>
        <div class="list">
          <div class="workspaceBar">
            <div class="workspaceLeft">
              <span class="workspaceTitle">Workspace</span>
              <select v-model="workspaceFilter">
                <option value="">All</option>
                <optgroup v-if="pinnedWorkspaces.length" label="Pinned">
                  <option
                    v-for="p in pinnedWorkspaces"
                    :key="'p-' + p"
                    :value="p"
                  >
                    {{ p }}
                  </option>
                </optgroup>
                <optgroup v-if="recentWorkspacesUnpinned.length" label="Recent">
                  <option
                    v-for="p in recentWorkspacesUnpinned"
                    :key="'r-' + p"
                    :value="p"
                  >
                    {{ p }}
                  </option>
                </optgroup>
              </select>
            </div>
            <button
              type="button"
              @click="setWorkspace(newWorkdir)"
              :disabled="!newWorkdir.trim()"
            >
              Use Workdir
            </button>
            <button
              type="button"
              @click="pinWorkspace(workspaceFilter || newWorkdir)"
              :disabled="!(workspaceFilter || newWorkdir).trim()"
            >
              Pin
            </button>
            <button
              type="button"
              @click="clearWorkspace"
              :disabled="!workspaceFilter"
            >
              All
            </button>
          </div>

          <div class="sessionSearchRow">
            <input
              v-model="sessionSearch"
              placeholder="Search sessions (id/workdir/prompt/status)..."
            />
            <button
              type="button"
              @click="sessionSearch = ''"
              :disabled="!sessionSearch.trim()"
              title="Clear"
            >
              ✕
            </button>
          </div>

          <div v-if="pinnedWorkspaces.length" class="pinnedWorkspaces">
            <div v-for="p in pinnedWorkspaces" :key="p" class="pinnedItem">
              <button
                type="button"
                class="pinnedBtn"
                :class="{ active: workspaceFilter === p }"
                @click="setWorkspace(p)"
                :title="p"
              >
                <span class="mono">{{ p }}</span>
              </button>
              <button
                type="button"
                class="pinnedX"
                @click="unpinWorkspace(p)"
                title="Unpin"
              >
                ✕
              </button>
            </div>
          </div>

          <div v-if="workspaceFilter" class="listMeta">
            Showing {{ filteredSessions.length }} /
            {{ sessionsAll.length }} sessions
          </div>

          <button
            v-for="s in filteredSessions"
            :key="s.key"
            class="row"
            :class="{ active: s.key === selectedSessionKey }"
            @click="onSelectTask(s.latest.id)"
          >
            <div class="rowTop">
              <span class="mono" :title="s.session_id || s.latest.id">{{
                (s.session_id || s.latest.id).slice(0, 8)
              }}</span>
              <span class="pill" :class="s.status">{{ s.status }}</span>
            </div>
            <div class="rowMid">
              <span class="pill kind">{{ s.worker_type }}</span>
              <span class="score">score {{ s.score }}</span>
              <span class="pill kind">{{ s.runs.length }} runs</span>
              <span v-if="s.warning" class="warn">⚠</span>
            </div>
            <div class="rowPath mono" :title="s.workdir">{{ s.workdir }}</div>
            <div class="rowBottom">{{ s.latest.prompt }}</div>
          </button>
        </div>
      </section>

      <section class="panel">
        <h2>Session Detail</h2>
        <div v-if="!selectedSession" class="empty">Select a session</div>
        <div v-else class="detail">
          <div class="detailHeader">
            <div class="detailTop">
              <div class="detailTopLeft">
                <span
                  class="mono detailSid"
                  :title="selectedSession.session_id || selectedSession.latest.id"
                  >{{
                    (selectedSession.session_id || selectedSession.latest.id).slice(0, 8)
                  }}</span
                >
                <span class="pill" :class="selectedSession.status">{{
                  selectedSession.status
                }}</span>
                <span class="pill kind">{{ selectedSession.worker_type }}</span>
                <span class="detailMini">
                  score <span class="score">{{ selectedSession.score }}</span>
                </span>
                <span class="detailMini">{{ selectedSession.runs.length }} runs</span>
                <span
                  v-if="selectedSession.warning"
                  class="warn"
                  :title="selectedSession.warning"
                  >⚠</span
                >
                <span
                  v-if="selectedTask?.error"
                  class="warn"
                  :title="selectedTask.error"
                  >!</span
                >
              </div>
              <div class="detailTopActions">
                <button
                  v-if="selectedTask?.status === 'running'"
                  type="button"
                  @click="onCancelTask"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  @click="setWorkspace(selectedSession.workdir)"
                  title="Focus workspace"
                >
                  Focus
                </button>
                <button
                  type="button"
                  @click="copyText(selectedSession.workdir)"
                  title="Copy workdir"
                >
                  Copy
                </button>
              </div>
            </div>

            <div class="detailWorkdir">
              <span class="mono detailWorkdirText" :title="selectedSession.workdir">{{
                selectedSession.workdir
              }}</span>
            </div>

            <details class="detailMore">
              <summary>More</summary>
              <div class="detailMoreGrid">
                <div>
                  <span class="k">Session</span>
                  <span class="mono">{{
                    selectedSession.session_id || "(pending)"
                  }}</span>
                </div>
                <div>
                  <span class="k">Status</span> {{ selectedSession.status }}
                </div>
                <div>
                  <span class="k">Score</span> {{ selectedSession.score }} (stderr
                  {{ selectedSession.stderr_count }})
                </div>
                <div>
                  <span class="k">Runs</span> {{ selectedSession.runs.length }}
                </div>
                <div v-if="selectedSession.warning" class="full">
                  <span class="k">Warning</span> {{ selectedSession.warning }}
                </div>
                <div v-if="selectedTask?.error" class="full">
                  <span class="k">Last Err</span> {{ selectedTask.error }}
                </div>
              </div>
            </details>
          </div>

          <div class="resumeBar">
            <div class="resumeRow">
              <input
                v-if="!resumeExpanded"
                v-model="resumePrompt"
                placeholder="Continue with..."
                @keydown.enter.prevent="onResumeTask"
              />
              <textarea
                v-else
                v-model="resumePrompt"
                rows="3"
                placeholder="Continue with..."
              ></textarea>
              <button
                type="button"
                class="primary"
                @click="onResumeTask"
                :disabled="!resumePrompt.trim() || !selectedSession.session_id"
              >
                Resume
              </button>
              <button
                type="button"
                @click="resumeExpanded = !resumeExpanded"
                :title="resumeExpanded ? 'Collapse' : 'Expand'"
              >
                {{ resumeExpanded ? "Less" : "More" }}
              </button>
            </div>
            <div v-if="!selectedSession.session_id" class="tinyHint">
              session_id pending：暂时无法 resume
            </div>
          </div>

          <div class="runs">
            <div class="runsHeader">
              Runs <span class="runsCount">{{ selectedSession.runs.length }}</span>
            </div>
            <div class="runList">
              <button
                v-for="r in selectedSession.runs.slice().reverse()"
                :key="r.id"
                type="button"
                class="runRow"
                :class="{ active: r.id === selectedTaskId }"
                @click="onSelectTask(r.id)"
              >
                <div class="runTop">
                  <span class="mono">{{ r.id.slice(0, 8) }}</span>
                  <span class="pill" :class="r.status">{{ r.status }}</span>
                </div>
                <div class="runMid">
                  <span class="pill kind">{{ r.mode }}</span>
                  <span class="score">score {{ r.score }}</span>
                  <span class="mono">{{ r.created_at }}</span>
                </div>
                <div class="runBottom">{{ r.prompt }}</div>
              </button>
            </div>
          </div>

          <div class="logs">
            <div class="outputTabs">
              <button
                type="button"
                class="tabBtn"
                :class="{ active: outputTab === 'result' }"
                @click="outputTab = 'result'"
              >
                Result
              </button>
              <button
                type="button"
                class="tabBtn"
                :class="{ active: outputTab === 'logs' }"
                @click="outputTab = 'logs'"
              >
                Logs
              </button>
              <div class="tabSpacer"></div>
              <button
                v-if="outputTab === 'result'"
                type="button"
                @click="copySelectedResult"
                :disabled="!selectedResultText"
              >
                Copy
              </button>
            </div>

            <div v-if="outputTab === 'result'" class="resultPanel">
              <div v-if="!selectedResultText" class="empty">
                {{
                  selectedTask?.status === "running" ||
                  selectedTask?.status === "queued"
                    ? "Task is running…"
                    : "No result yet."
                }}
              </div>
              <pre v-else class="resultBox">{{ selectedResultText }}</pre>
            </div>

            <div v-else class="logsPanel">
              <div class="logControls">
                <div class="logFilters">
                  <label class="logFilter">
                    <input type="checkbox" v-model="logShowAssistant" />
                    assistant
                  </label>
                  <label class="logFilter">
                    <input type="checkbox" v-model="logShowStdout" />
                    stdout
                  </label>
                  <label class="logFilter">
                    <input type="checkbox" v-model="logShowStderr" />
                    stderr
                  </label>
                  <label class="logFilter">
                    <input type="checkbox" v-model="logShowSystem" />
                    system
                  </label>
                </div>
                <div class="logMeta">
                  {{ filteredLogs.length }} / {{ selectedLogs.length }}
                </div>
                <input v-model="logSearch" placeholder="Filter logs..." />
              </div>

              <div class="logbox">
                <div
                  v-for="l in filteredLogs"
                  :key="l.id"
                  class="logLine"
                  :class="`s-${l.stream}`"
                >
                  <span class="logTime">{{ formatLogTime(l.time) }}</span>
                  <span class="logTag" :class="l.stream">{{ l.stream }}</span>
                  <span class="logMsg">{{ l.message }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <button
      type="button"
      class="secOrb"
      :class="{
        open: secretaryOpen,
        attention: needsAttentionSessions.length > 0,
      }"
      @click="toggleSecretary"
      :title="secretaryOpen ? 'Close secretary (S)' : 'Open secretary (S)'"
      aria-label="Secretary"
    >
      <span class="secOrbIcon">S</span>
      <span v-if="needsAttentionSessions.length" class="secOrbBadge">{{
        needsAttentionSessions.length
      }}</span>
    </button>

    <div
      v-if="secretaryOpen"
      class="secDrawerOverlay"
      @click.self="closeSecretary"
    >
      <aside class="secDrawer" role="dialog" aria-modal="true">
        <div class="secDrawerHeader">
          <div class="secDrawerTitle">Secretary</div>
          <div class="secTabs" role="tablist" aria-label="Secretary tabs">
            <button
              type="button"
              class="secTab"
              :class="{ active: secretaryView === 'chat' }"
              role="tab"
              :aria-selected="secretaryView === 'chat'"
              @click="secretaryView = 'chat'"
            >
              Chat
            </button>
            <button
              type="button"
              class="secTab"
              :class="{ active: secretaryView === 'overview' }"
              role="tab"
              :aria-selected="secretaryView === 'overview'"
              @click="secretaryView = 'overview'"
            >
              Overview
              <span
                v-if="needsAttentionSessions.length"
                class="secTabBadge"
                :title="`Needs attention: ${needsAttentionSessions.length}`"
              >
                {{ needsAttentionSessions.length }}
              </span>
            </button>
            <button
              type="button"
              class="secTab"
              :class="{ active: secretaryView === 'feed' }"
              role="tab"
              :aria-selected="secretaryView === 'feed'"
              @click="secretaryView = 'feed'"
            >
              Feed
              <span class="secTabBadge" :title="`Feed items: ${feedItems.length}`">
                {{ feedItems.length }}
              </span>
            </button>
          </div>
          <button class="iconBtn" type="button" @click="closeSecretary">
            ✕
          </button>
        </div>

        <div class="secDrawerBody">
          <div v-if="secretaryView === 'feed'" class="secFeed">
            <div class="feedControls">
              <div class="feedLeft">
                <label class="feedLabel">
                  Scope
                  <select v-model="feedScope">
                    <option value="current">Current</option>
                    <option value="all">All</option>
                  </select>
                </label>
                <label class="feedToggle">
                  <input type="checkbox" v-model="feedWrap" />
                  Wrap
                </label>
                <button type="button" @click="feedPaused = !feedPaused">
                  {{ feedPaused ? "Resume" : "Pause" }}
                </button>
              </div>
              <div class="feedRight">
                <span
                  v-if="selectedTask?.status === 'running' && feedIdleSeconds >= 10"
                  class="feedIdle"
                  :title="`No output for ${feedIdleSeconds}s`"
                >
                  No output {{ feedIdleSeconds }}s
                </span>
              </div>
            </div>

            <div
              ref="feedBoxEl"
              class="feedBox"
              :class="{ wrap: feedWrap }"
              role="log"
              aria-label="Live feed"
            >
              <div v-if="feedItems.length === 0" class="empty">
                暂无日志（仅展示本次打开页面后收到的实时日志）
              </div>
              <div v-else class="feedLines">
                <div v-for="(f, idx) in feedItems" :key="f.task_id + ':' + f.time + ':' + idx" class="feedLine">
                  <span class="feedTime mono">{{ formatLogTime(f.time) }}</span>
                  <span class="feedTask mono" :title="f.task_id">{{ f.task_short }}</span>
                  <span class="feedStream">{{ f.stream }}</span>
                  <span class="feedMsg">{{ f.message }}</span>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="secretaryView === 'overview'" class="secOverview">
            <div class="secretaryCards">
              <div class="secCard">
                <div class="secK">Sessions</div>
                <div class="secV">{{ secretaryCounts.total }}</div>
              </div>
              <div class="secCard">
                <div class="secK">Running</div>
                <div class="secV">{{ secretaryCounts.running }}</div>
              </div>
              <div class="secCard">
                <div class="secK">Blocked</div>
                <div class="secV">{{ secretaryCounts.blocked }}</div>
              </div>
              <div class="secCard">
                <div class="secK">Failed</div>
                <div class="secV">{{ secretaryCounts.failed }}</div>
              </div>
            </div>

            <div class="secSection">
              <div class="secSectionTitle">Needs Attention</div>
              <div v-if="needsAttentionSessions.length === 0" class="empty">
                暂无需要关注的 session
              </div>
              <button
                v-for="s in needsAttentionSessions"
                :key="s.key"
                type="button"
                class="secRow"
                @click="
                  onSelectTask(s.latest.id);
                  closeSecretary();
                "
              >
                <div class="rowTop">
                  <span class="mono">{{
                    (s.session_id || s.latest.id).slice(0, 8)
                  }}</span>
                  <span class="pill" :class="s.status">{{ s.status }}</span>
                </div>
                <div class="rowMid">
                  <span class="pill kind">{{ s.worker_type }}</span>
                  <span class="score">score {{ s.score }}</span>
                </div>
                <div class="rowPath mono">{{ s.workdir }}</div>
              </button>
            </div>

            <div class="secSection">
              <div class="secSectionTitle">Briefing</div>
              <pre class="briefing">{{ secretaryBriefing }}</pre>
            </div>
          </div>

          <div v-else class="secChatView">
            <div
              v-if="needsAttentionSessions.length"
              class="secAttentionHint"
            >
              <div class="text">
                Needs attention: {{ needsAttentionSessions.length }} session(s)
              </div>
              <button
                type="button"
                @click="secretaryView = 'overview'"
                title="Open overview"
              >
                View
              </button>
            </div>

            <div class="secChat">
              <div class="chat">
                <div class="chatControls">
                  <label>
                    Agent
                    <select v-model="chatBackend" :disabled="chatSending">
                      <option value="auto">auto</option>
                      <option value="claude">claude</option>
                      <option value="codex">codex</option>
                    </select>
                  </label>
                  <label class="chatToggle">
                    <input
                      type="checkbox"
                      v-model="chatStreamEnabled"
                      :disabled="chatSending"
                    />
                    Stream
                  </label>
                  <label>
                    Max steps
                    <input
                      type="number"
                      min="1"
                      max="32"
                      v-model.number="chatMaxSteps"
                      :disabled="chatSending"
                    />
                  </label>
                </div>
                <div class="msgs">
                  <div
                    v-for="m in chat"
                    :key="m.id"
                    class="msg"
                    :class="m.role"
                  >
                    <div class="role">{{ m.role }}</div>
                    <div class="content">{{ m.content }}</div>
                  </div>
                  <div
                    v-if="chatStreamStatus || chatStreamAnswer"
                    class="msg assistant streaming"
                  >
                    <div class="role">assistant</div>
                    <div class="content">
                      {{ chatStreamAnswer || chatStreamStatus }}
                    </div>
                  </div>
                </div>
                <div class="input">
                  <textarea
                    ref="chatInputEl"
                    v-model="chatInput"
                    rows="3"
                    placeholder="Ask the secretary..."
                  ></textarea>
                  <button
                    class="primary"
                    @click="onSendChat"
                    :disabled="chatSending || !chatInput.trim()"
                  >
                    Send
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </aside>
    </div>

    <div
      v-if="newRunOpen"
      class="modalOverlay"
      @click.self="closeNewRun"
    >
      <div class="modal newRunModal">
        <div class="modalHeader">
          <div class="modalTitle">New Run</div>
          <button class="iconBtn" type="button" @click="closeNewRun">✕</button>
        </div>

        <div class="modalBody newRunBody">
          <div class="form newRunForm">
            <label>
              Worker
              <select v-model="newWorkerType">
                <option value="claude-code">claude-code</option>
                <option value="codex">codex</option>
              </select>
            </label>
            <label>
              Workdir
              <div class="workdirRow">
                <input v-model="newWorkdir" placeholder="." />
                <button type="button" @click="openDirPicker">Browse</button>
              </div>
            </label>
            <div v-if="missingAuthText" class="authHint full">
              <div class="text">{{ missingAuthText }}</div>
              <button type="button" @click="openAuthSettings">
                Auth Settings
              </button>
            </div>
            <label class="full">
              Prompt
              <textarea
                ref="newRunPromptEl"
                v-model="newPrompt"
                rows="6"
                placeholder="Describe the task to run..."
              ></textarea>
            </label>
            <div class="newRunHint full">
              Hotkeys: <span class="mono">N</span> new run ·
              <span class="mono">S</span> secretary ·
              <span class="mono">Esc</span> close
            </div>
          </div>
        </div>

        <div class="modalFooter">
          <button type="button" @click="closeNewRun">Cancel</button>
          <button
            type="button"
            class="primary"
            @click="onCreateTaskFromModal"
            :disabled="
              !newPrompt.trim() || !newWorkdir.trim() || !!missingAuthText
            "
          >
            Start
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="authSettingsOpen"
      class="modalOverlay"
      @click.self="authSettingsOpen = false"
    >
      <div class="modal settingsModal">
        <div class="modalHeader">
          <div class="modalTitle">Auth Settings</div>
          <button
            class="iconBtn"
            type="button"
            @click="authSettingsOpen = false"
          >
            ✕
          </button>
        </div>

        <div class="modalBody settingsBody">
          <div class="settingsMeta" v-if="authInfo?.storage_path">
            Storage: <span class="mono">{{ authInfo.storage_path }}</span>
          </div>

          <div v-if="authSettingsError" class="modalError">
            {{ authSettingsError }}
          </div>

          <div class="settingsSection">
            <div class="settingsSectionTitle">Claude Code</div>
            <div class="kv">
              <span class="k">ANTHROPIC_BASE_URL</span>
              <span class="mono"
                >{{ authStatus?.claude.base_url.effective }}
                {{ authStatus?.claude.base_url.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_API_KEY</span>
              <span class="mono"
                >{{ authStatus?.claude.api_key.effective }}
                {{ authStatus?.claude.api_key.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_AUTH_TOKEN</span>
              <span class="mono"
                >{{ authStatus?.claude.auth_token.effective }}
                {{ authStatus?.claude.auth_token.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_MODEL</span>
              <span class="mono"
                >{{ authStatus?.claude.model.effective }}
                {{ authStatus?.claude.model.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_SMALL_FAST_MODEL</span>
              <span class="mono"
                >{{ authStatus?.claude.small_fast_model.effective }}
                {{ authStatus?.claude.small_fast_model.masked }}</span
              >
            </div>

            <label class="full">
              Store ANTHROPIC_BASE_URL
              <div class="secretRow">
                <input
                  v-model="authAnthropicBaseURL"
                  placeholder="https://..."
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('anthropic_base_url')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_API_KEY
              <div class="secretRow">
                <input
                  v-model="authAnthropicApiKey"
                  type="password"
                  placeholder="Paste key…"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('anthropic_api_key')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_AUTH_TOKEN
              <div class="secretRow">
                <input
                  v-model="authAnthropicAuthToken"
                  type="password"
                  placeholder="Paste token…"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('anthropic_auth_token')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_MODEL
              <div class="secretRow">
                <input
                  v-model="authAnthropicModel"
                  placeholder="model name…"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('anthropic_model')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_SMALL_FAST_MODEL
              <div class="secretRow">
                <input
                  v-model="authAnthropicSmallFastModel"
                  placeholder="model name…"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('anthropic_small_fast_model')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>

            <div class="settingsHelp">
              如果你使用 Claude Code 订阅登录模式，也可以在终端运行一次
              <span class="mono">claude /login</span>。
            </div>
          </div>

          <div class="settingsSection">
            <div class="settingsSectionTitle">Codex</div>
            <div class="kv">
              <span class="k">OPENAI_API_KEY</span>
              <span class="mono"
                >{{ authStatus?.codex.api_key.effective }}
                {{ authStatus?.codex.api_key.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">MODEL</span>
              <span class="mono"
                >{{ authStatus?.codex.model.effective }}
                {{ authStatus?.codex.model.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">REASONING</span>
              <span class="mono"
                >{{ authStatus?.codex.reasoning_effort.effective }}
                {{ authStatus?.codex.reasoning_effort.masked }}</span
              >
            </div>
            <label class="full">
              Store OPENAI_API_KEY
              <div class="secretRow">
                <input
                  v-model="authOpenAIApiKey"
                  type="password"
                  placeholder="Paste key…"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('openai_api_key')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Set model (default gpt-5.2)
              <div class="secretRow">
                <input
                  v-model="authCodexModel"
                  placeholder="gpt-5.2"
                  autocomplete="off"
                />
                <button
                  type="button"
                  @click="clearStoredAuth('codex_model')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Set reasoning effort (default xhigh)
              <div class="secretRow">
                <select v-model="authCodexReasoningEffort">
                  <option value="">(keep)</option>
                  <option value="low">low</option>
                  <option value="medium">medium</option>
                  <option value="high">high</option>
                  <option value="xhigh">xhigh</option>
                </select>
                <button
                  type="button"
                  @click="clearStoredAuth('codex_reasoning_effort')"
                  :disabled="authSaving"
                >
                  Clear stored
                </button>
              </div>
            </label>
          </div>
        </div>

        <div class="modalFooter">
          <button type="button" @click="authSettingsOpen = false">Close</button>
          <button
            type="button"
            class="primary"
            @click="saveAuthSettings"
            :disabled="authSaving"
          >
            {{ authSaving ? "Saving..." : "Save" }}
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="dirPickerOpen"
      class="modalOverlay"
      @click.self="dirPickerOpen = false"
    >
      <div class="modal">
        <div class="modalHeader">
          <div class="modalTitle">Select folder</div>
          <button class="iconBtn" type="button" @click="dirPickerOpen = false">
            ✕
          </button>
        </div>

        <div class="modalBody dirModalBody">
          <div class="roots">
            <button
              v-for="r in dirRoots"
              :key="r.path"
              type="button"
              class="rootBtn"
              @click="loadDir(r.path)"
            >
              {{ r.name }}
            </button>
          </div>

          <div class="pathRow">
            <button
              type="button"
              @click="dirParent && loadDir(dirParent)"
              :disabled="!dirParent"
            >
              Up
            </button>
            <div class="path mono">{{ dirPath }}</div>
            <button
              type="button"
              class="primary"
              @click="selectDir(dirPath)"
              :disabled="!dirPath"
            >
              Select
            </button>
          </div>

          <div v-if="dirError" class="modalError">{{ dirError }}</div>

          <div class="filterRow">
            <input v-model="dirFilter" placeholder="Filter folders..." />
            <span v-if="dirLoading" class="loading">Loading...</span>
          </div>

          <div class="dirList">
            <button
              v-for="e in filteredDirEntries"
              :key="e.path"
              type="button"
              class="dirItem"
              @click="loadDir(e.path)"
            >
              <span class="mono">📁</span>
              <span class="name">{{ e.name }}</span>
            </button>
          </div>
        </div>

        <div class="modalFooter">
          <button type="button" @click="dirPickerOpen = false">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
:global(:root) {
  --bg-app: #f1f5f9; /* Slate 100 - slightly darker than before for contrast */
  --bg-panel: #ffffff;
  --bg-subtle: #f8fafc;
  --color-primary: #0d9488; /* Teal 600 */
  --color-primary-hover: #0f766e; /* Teal 700 */
  --color-primary-bg: #ccfbf1; /* Teal 100 */
  --text-main: #334155; /* Slate 700 */
  --text-sub: #64748b; /* Slate 500 */
  --border-color: #e2e8f0;
  --bg-header: rgba(255, 255, 255, 0.9);
  --bg-header-border: rgba(255, 255, 255, 0.5);
  --overlay-modal: rgba(240, 253, 250, 0.6);
  --overlay-drawer: rgba(15, 23, 42, 0.35);
  --bg-card-active-a: #f0fdfa;
  --bg-card-active-b: #e0f2fe;
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.06), 0 2px 4px -2px rgb(0 0 0 / 0.06);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.05), 0 4px 6px -4px rgb(0 0 0 / 0.05);
  --font-main: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}

:global(:root[data-theme="dark"]) {
  --bg-app: #0b1220;
  --bg-panel: #0f172a;
  --bg-subtle: #111c33;
  --color-primary: #2dd4bf;
  --color-primary-hover: #14b8a6;
  --color-primary-bg: rgba(45, 212, 191, 0.14);
  --text-main: #e5e7eb;
  --text-sub: #94a3b8;
  --border-color: rgba(148, 163, 184, 0.22);
  --bg-header: rgba(15, 23, 42, 0.9);
  --bg-header-border: rgba(148, 163, 184, 0.12);
  --overlay-modal: rgba(2, 6, 23, 0.55);
  --overlay-drawer: rgba(2, 6, 23, 0.55);
  --bg-card-active-a: rgba(45, 212, 191, 0.16);
  --bg-card-active-b: rgba(56, 189, 248, 0.12);
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.35);
  --shadow-md: 0 6px 10px -1px rgb(0 0 0 / 0.35), 0 2px 6px -2px rgb(0 0 0 / 0.25);
  --shadow-lg: 0 14px 22px -6px rgb(0 0 0 / 0.55), 0 10px 10px -8px rgb(0 0 0 / 0.35);
}

:global(html),
:global(body) {
  margin: 0;
  padding: 0;
  background: var(--bg-app);
  color: var(--text-main);
}

.page {
  font-family: var(--font-main);
  color: var(--text-main);
  background: linear-gradient(180deg, var(--bg-subtle) 0%, var(--bg-app) 100%);
  min-height: 100vh;
  box-sizing: border-box;
  padding-bottom: 20px;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 32px;
  background: var(--bg-header);
  backdrop-filter: blur(12px);
  position: sticky;
  top: 0;
  z-index: 50;
  border-bottom: 1px solid var(--bg-header-border);
  box-shadow: var(--shadow-sm);
  margin-bottom: 24px;
}

.headerRight {
  display: flex;
  align-items: center;
  gap: 12px;
}

.title {
  font-weight: 800;
  font-size: 20px;
  background: linear-gradient(135deg, #0d9488 0%, #0ea5e9 100%);
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
}

.sub {
  color: var(--text-sub);
  font-size: 12px;
  font-weight: 500;
  background: var(--bg-app);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
}

.settingsBtn {
  border: none;
  background: transparent;
  color: var(--text-sub);
  font-weight: 600;
  font-size: 13px;
  cursor: pointer;
  transition: color 0.2s;
}
.settingsBtn:hover {
  color: var(--color-primary);
}

.themeBtn {
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  color: var(--text-sub);
  font-weight: 800;
  font-size: 12px;
  border-radius: 999px;
  padding: 6px 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.themeBtn:hover {
  color: var(--color-primary);
  border-color: rgba(45, 212, 191, 0.35);
  background: var(--bg-subtle);
}

.banner {
  margin: 0 24px 20px;
  background: #fef2f2;
  border: 1px solid #fee2e2;
  color: #ef4444;
  padding: 12px 16px;
  border-radius: var(--radius-md);
  font-size: 13px;
  font-weight: 500;
  box-shadow: var(--shadow-sm);
}

.grid {
  display: grid;
  grid-template-columns:
    minmax(340px, 1fr)
    minmax(560px, 2fr);
  gap: 24px;
  padding: 0 clamp(16px, 2vw, 32px);
  width: 100%;
  box-sizing: border-box;
  max-width: 3200px;
  margin: 0 auto;
  align-items: start; /* Important for sticky to work */
}

.panel {
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 0; /* Removing padding from parent, moved to children */
  min-height: 200px;
  min-width: 0;
  box-shadow: var(--shadow-md);
  display: flex;
  flex-direction: column;
  transition: box-shadow 0.3s ease;
  overflow: hidden; /* For header radius */
}

/* Sticky sidebars */
.grid > section:first-child {
  position: sticky;
  top: 90px; /* Header height + spacing */
  max-height: calc(100vh - 110px);
  overflow-y: auto;
}

.panel:hover {
  box-shadow: var(--shadow-lg);
}

h2 {
  margin: 0;
  padding: 16px 20px;
  font-size: 15px;
  font-weight: 700;
  color: white;
  background: linear-gradient(135deg, #0d9488 0%, #0891b2 100%);
  display: flex;
  align-items: center;
  gap: 8px;
  text-shadow: 0 1px 2px rgba(0,0,0,0.1);
}

.form, .list, .detail, .secretary {
  padding: 20px;
}

.form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 20px;
}

.form > button.primary {
  grid-column: 1 / -1;
  padding: 12px 16px;
  font-size: 14px;
  font-weight: 700;
}

.form label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-main);
}

.form .full {
  grid-column: 1 / -1;
}

.authHint {
  grid-column: 1 / -1;
  display: flex;
  gap: 12px;
  align-items: center;
  background: #fff7ed;
  border: 1px solid #ffedd5;
  color: #c2410c;
  padding: 12px;
  border-radius: var(--radius-md);
  font-size: 13px;
}

:global(:root[data-theme="dark"]) .authHint,
:global(:root[data-theme="dark"]) .secAttentionHint {
  background: rgba(251, 146, 60, 0.12);
  border-color: rgba(251, 146, 60, 0.22);
  color: #fdba74;
}

.authHint .text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

input,
select,
textarea {
  border: 1px solid var(--border-color);
  background-color: var(--bg-subtle);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  font-size: 14px;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, background-color 0.15s ease;
  color: var(--text-main);
  font-family: var(--font-main);
  width: 100%;
  box-sizing: border-box;
}

input:focus,
select:focus,
textarea:focus {
  border-color: var(--color-primary);
  background: var(--bg-panel);
  box-shadow: 0 0 0 3px var(--color-primary-bg);
}

textarea {
  resize: vertical; /* Only allow vertical resize */
  line-height: 1.6;
  min-height: 92px;
  max-height: 60vh;
  background-color: var(--bg-panel);
  padding: 12px 14px;
  font-size: 14px;
  color: var(--text-main);
  font-family: var(--font-main);
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, background-color 0.15s ease;
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.04);
  overflow: auto;
  overflow-x: hidden;
  overscroll-behavior: contain;
}

textarea:hover {
  border-color: #94a3b8;
  background-color: var(--bg-panel);
}

textarea:focus {
  border-color: var(--color-primary);
  background-color: var(--bg-panel);
  box-shadow: 0 0 0 3px var(--color-primary-bg), inset 0 1px 2px rgba(0, 0, 0, 0.04);
}

textarea::placeholder {
  color: #94a3b8;
  font-style: italic;
}

button {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 8px 14px;
  background: var(--bg-panel);
  color: var(--text-main);
  font-weight: 500;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

button:hover:not(:disabled) {
  background: var(--bg-subtle);
  border-color: rgba(148, 163, 184, 0.5);
  color: var(--color-primary);
}

button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  background: var(--bg-subtle);
}

button.primary {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
  box-shadow: 0 2px 4px rgba(13, 148, 136, 0.2);
}

button.primary:disabled {
  background: #94a3b8;
  border-color: #94a3b8;
  opacity: 1;
  box-shadow: none;
}

button.primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
  border-color: var(--color-primary-hover);
  transform: translateY(-1px);
  box-shadow: 0 4px 6px rgba(13, 148, 136, 0.3);
  color: white;
}

button.primary:active:not(:disabled) {
  transform: translateY(0);
}

.workdirRow {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
}

.sessionSearchRow {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
}

.sessionSearchRow button {
  width: 44px;
  padding: 8px 0;
  border-radius: 12px;
}

.newRunBody {
  padding: 0;
}

.newRunForm {
  padding: 20px;
  margin: 0;
}

.newRunHint {
  font-size: 12px;
  color: var(--text-sub);
  border-top: 1px solid var(--border-color);
  padding-top: 12px;
}

.secOrb {
  position: fixed;
  right: -18px;
  bottom: 26px;
  width: 56px;
  height: 56px;
  border-radius: 999px;
  border: none;
  background: linear-gradient(135deg, #0d9488 0%, #0891b2 100%);
  color: white;
  box-shadow: var(--shadow-lg);
  display: grid;
  place-items: center;
  cursor: pointer;
  transition:
    right 0.2s ease,
    transform 0.2s ease,
    box-shadow 0.2s ease;
  z-index: 250;
}

.secOrb:hover,
.secOrb.open {
  right: 16px;
}

.secOrb:hover {
  transform: translateY(-2px);
}

.secOrb.attention {
  background: linear-gradient(135deg, #f97316 0%, #ef4444 100%);
}

.secOrbIcon {
  font-weight: 900;
  letter-spacing: -0.02em;
}

.secOrbBadge {
  position: absolute;
  top: -6px;
  right: 6px;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 999px;
  background: #ef4444;
  color: white;
  border: 2px solid var(--bg-panel);
  font-size: 12px;
  font-weight: 900;
  display: grid;
  place-items: center;
  line-height: 1;
}

.secDrawerOverlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-drawer);
  backdrop-filter: blur(2px);
  z-index: 200;
}

.secDrawer {
  position: fixed;
  top: 90px;
  right: 16px;
  bottom: 16px;
  width: min(440px, calc(100vw - 32px));
  background: var(--bg-panel);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--border-color);
  overflow: hidden;
  display: grid;
  grid-template-rows: auto 1fr;
}

.secDrawerHeader {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

.secDrawerTitle {
  font-weight: 800;
  font-size: 14px;
  color: var(--text-main);
}

.secTabs {
  display: flex;
  gap: 6px;
  flex: 1;
  justify-content: center;
}

.secTab {
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
  position: relative;
}

.secTab.active {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

.secTabBadge {
  margin-left: 6px;
  display: inline-grid;
  place-items: center;
  min-width: 18px;
  height: 18px;
  padding: 0 6px;
  border-radius: 999px;
  background: rgba(239, 68, 68, 0.12);
  border: 1px solid rgba(239, 68, 68, 0.25);
  color: #ef4444;
  font-size: 11px;
  font-weight: 900;
}

.secDrawerBody {
  padding: 16px;
  overflow: auto;
}

.secOverview {
  display: grid;
  gap: 20px;
}

.secChatView {
  display: grid;
  gap: 12px;
}

.secAttentionHint {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #fff7ed;
  border: 1px solid #ffedd5;
  color: #c2410c;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  font-size: 13px;
}

.secAttentionHint .text {
  flex: 1;
}

.secFeed {
  display: grid;
  gap: 12px;
  height: calc(100vh - 170px);
  max-height: 900px;
}

.feedControls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.feedLeft {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}

.feedLabel {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
  color: var(--text-sub);
}

.feedLabel select {
  padding: 8px 10px;
  font-size: 13px;
}

.feedToggle {
  display: flex;
  gap: 8px;
  align-items: center;
  padding-bottom: 8px;
  font-size: 12px;
  color: var(--text-sub);
}

.feedRight {
  display: flex;
  align-items: center;
  gap: 10px;
}

.feedIdle {
  font-size: 12px;
  font-weight: 900;
  color: #fdba74;
  background: rgba(251, 146, 60, 0.12);
  border: 1px solid rgba(251, 146, 60, 0.22);
  padding: 6px 10px;
  border-radius: 999px;
}

.feedBox {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  overflow: auto;
  padding: 10px;
  min-height: 0;
  box-shadow: inset 0 2px 4px rgba(0, 0, 0, 0.08);
}

.feedLines {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.feedLine {
  display: grid;
  grid-template-columns: 60px 64px 70px 1fr;
  gap: 10px;
  align-items: start;
  padding: 6px 6px;
  border-radius: 10px;
}

.feedLine:hover {
  background: rgba(148, 163, 184, 0.08);
}

.feedTime {
  font-size: 11px;
  color: var(--text-sub);
}

.feedTask {
  font-size: 11px;
  color: var(--text-sub);
}

.feedStream {
  font-size: 11px;
  font-weight: 800;
  color: var(--color-primary);
  text-transform: lowercase;
}

.feedMsg {
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-main);
  white-space: pre-wrap;
  word-break: break-word;
}

.feedBox:not(.wrap) .feedMsg {
  white-space: pre;
  overflow: hidden;
  text-overflow: ellipsis;
}

.modalOverlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-modal);
  backdrop-filter: blur(4px);
  display: grid;
  place-items: center;
  padding: 24px;
  z-index: 999;
}

.modal, .settingsModal {
  background: var(--bg-panel);
  border-radius: 24px;
  border: 1px solid var(--border-color);
  box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1);
  overflow: hidden;
  animation: popIn 0.2s ease-out;
}

.modal {
  width: min(860px, 95vw);
  height: min(600px, 90vh);
  display: grid;
  grid-template-rows: auto 1fr auto;
}

.settingsModal {
  width: min(760px, 95vw);
  height: min(600px, 90vh);
}

@keyframes popIn {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}

.modalHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

.modalFooter {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px;
  border-top: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

.modalTitle {
  font-weight: 700;
  font-size: 16px;
  color: var(--text-main);
}

.iconBtn {
  border: none;
  background: transparent;
  padding: 8px;
  color: var(--text-sub);
  border-radius: 50%;
}
.iconBtn:hover {
  background: var(--bg-subtle);
  color: var(--text-main);
}

.modalBody, .settingsBody, .dirModalBody {
  padding: 20px;
  min-height: 0;
}

.settingsModal .modalBody {
  overflow: auto;
}

.dirModalBody {
  display: grid;
  grid-template-rows: auto auto auto auto 1fr;
  gap: 16px;
}

.settingsBody {
  display: grid;
  gap: 20px;
}

.settingsSection {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 16px;
  display: grid;
  gap: 12px;
  background: var(--bg-subtle);
}

.settingsSectionTitle {
  font-weight: 700;
  font-size: 14px;
  color: var(--color-primary);
}

.kv {
  display: grid;
  grid-template-columns: 180px 1fr;
  gap: 12px;
  align-items: center;
  font-size: 13px;
}

.secretRow {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
}

.roots {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.rootBtn {
  font-size: 13px;
  padding: 6px 14px;
  border-radius: 999px;
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  color: var(--text-main);
}

.rootBtn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
  background: var(--color-primary-bg);
}

.pathRow {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 12px;
  align-items: center;
}

.path {
  padding: 10px 14px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-subtle);
  color: var(--text-sub);
  font-family: var(--font-mono);
  font-size: 13px;
}

.dirList {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
  padding: 8px;
  background: var(--bg-panel);
  max-height: 100%;
}

.dirItem {
  width: 100%;
  display: grid;
  grid-template-columns: 24px 1fr;
  gap: 10px;
  align-items: center;
  text-align: left;
  border: 1px solid transparent;
  background: transparent;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  font-size: 14px;
  color: var(--text-main);
}

.dirItem:hover {
  background: var(--bg-subtle);
  color: var(--color-primary);
}

.list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 20px; /* match panel content padding */
}

.workspaceBar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 8px;
}

.workspaceLeft {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
}

.workspaceTitle {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.pinnedWorkspaces {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.pinnedItem {
  display: flex;
  align-items: stretch;
  border: 1px solid var(--border-color);
  border-radius: 999px;
  overflow: hidden;
  background: var(--bg-panel);
  box-shadow: var(--shadow-sm);
  transition: all 0.2s;
}

.pinnedItem:hover {
  box-shadow: var(--shadow-md);
  border-color: var(--color-primary);
}

.pinnedBtn {
  border: none;
  background: transparent;
  padding: 6px 14px;
  cursor: pointer;
  max-width: 220px;
  text-align: left;
  font-size: 12px;
}

.pinnedBtn.active {
  background: var(--color-primary);
  color: white;
}

.pinnedX {
  border: none;
  border-left: 1px solid var(--border-color);
  background: transparent;
  padding: 6px 10px;
  cursor: pointer;
  color: var(--text-sub);
}
.pinnedX:hover {
  color: #ef4444;
  background: #fef2f2;
}

.listMeta {
  font-size: 12px;
  color: var(--text-sub);
  margin-bottom: 4px;
  padding-left: 4px;
}

.row {
  text-align: left;
  background: linear-gradient(135deg, var(--bg-panel) 0%, var(--bg-subtle) 100%);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 2px 4px rgba(0,0,0,0.03);
  cursor: pointer;
  position: relative;
  overflow: hidden;
}

.row::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: transparent;
  transition: background 0.2s;
}

.row:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 16px -4px rgba(0,0,0,0.1);
  border-color: var(--color-primary-bg);
}

.row:hover::before {
  background: linear-gradient(180deg, #0d9488 0%, #0891b2 100%);
}

.row.active {
  border-color: var(--color-primary);
  background: linear-gradient(135deg, var(--bg-card-active-a) 0%, var(--bg-card-active-b) 100%);
  box-shadow: 0 0 0 3px rgba(13, 148, 136, 0.15);
}

.row.active::before {
  background: linear-gradient(180deg, #0d9488 0%, #0891b2 100%);
}

.rowTop {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}

.rowMid {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-sub);
  font-size: 12px;
  margin-bottom: 8px;
}

.rowPath {
  font-size: 12px;
  color: var(--text-sub);
  margin-bottom: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: var(--bg-subtle);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
}

.rowBottom {
  font-size: 13px;
  color: var(--text-main);
  display: -webkit-box;
  line-clamp: 2;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.5;
}

.mono {
  font-family: var(--font-mono);
  font-size: 0.9em;
}

.pill {
  font-size: 11px;
  padding: 2px 10px;
  border-radius: 999px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  border: 1px solid transparent;
}

.pill.kind {
  background: var(--bg-subtle);
  color: var(--text-sub);
}

.pill.running {
  background: linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%);
  color: white;
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.8; }
}

.pill.succeeded {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  color: white;
}

.pill.failed {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  color: white;
}

.pill.canceled,
.pill.interrupted {
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
  color: white;
}

.pill.queued {
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  color: white;
}

.pill.blocked {
  background: linear-gradient(135deg, #f97316 0%, #ea580c 100%);
  color: white;
}

.score {
  font-weight: 700;
  color: var(--color-primary);
}

.detail {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.detail .k {
  display: inline-block;
  width: 70px;
  color: var(--text-sub);
  font-weight: 600;
}

.detailHeader {
  display: grid;
  gap: 10px;
  background: var(--bg-panel);
  padding: 12px 14px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  margin-bottom: 12px;
  box-shadow: var(--shadow-sm);
}

.detailTop {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.detailTopLeft {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.detailSid {
  font-weight: 900;
  font-size: 14px;
}

.detailMini {
  font-size: 12px;
  color: var(--text-sub);
  font-weight: 700;
  background: var(--bg-subtle);
  padding: 4px 8px;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
}

.detailTopActions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.detailWorkdir {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.detailWorkdirText {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-main);
}

.detailMore {
  border-top: 1px solid var(--border-color);
  padding-top: 10px;
}

.detailMore summary {
  cursor: pointer;
  color: var(--color-primary);
  font-weight: 800;
  font-size: 12px;
  list-style: none;
}

.detailMore summary::-webkit-details-marker {
  display: none;
}

.detailMoreGrid {
  margin-top: 10px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 14px;
  font-size: 13px;
}

.detailMoreGrid .full {
  grid-column: 1 / -1;
}

.resumeBar {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
}

.resumeRow {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 10px;
  align-items: start;
}

.resumeRow textarea {
  min-height: 0;
}

.tinyHint {
  font-size: 12px;
  color: var(--text-sub);
}

.runs {
  display: grid;
  gap: 10px;
  margin-bottom: 16px;
  flex: 0 0 auto;
}

.runsHeader {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-main);
  display: flex;
  align-items: center;
  gap: 10px;
}

.runsCount {
  font-size: 12px;
  font-weight: 900;
  color: var(--text-sub);
  background: var(--bg-subtle);
  border: 1px solid rgba(148, 163, 184, 0.25);
  padding: 2px 10px;
  border-radius: 999px;
}

.runList {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
  padding: 10px;
  background: var(--bg-subtle);
  max-height: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.03);
}

.runRow {
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px 12px 10px;
  transition: all 0.2s;
  cursor: pointer;
  width: 100%;
  text-align: left;
}

.runRow:hover {
  border-color: var(--color-primary);
}
.runRow.active {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-bg);
}

.runTop {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.runMid {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  color: var(--text-sub);
  font-size: 12px;
  margin-bottom: 8px;
}

.runBottom {
  font-size: 13px;
  color: var(--text-main);
  line-height: 1.45;
  display: -webkit-box;
  line-clamp: 2;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.logs {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 400px; /* Ensure reasonable height */
}

.outputTabs {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
  flex: 0 0 auto;
}

.tabBtn {
  padding: 6px 12px;
  border-radius: 999px;
}

.tabBtn.active {
  border-color: var(--color-primary);
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.tabSpacer {
  flex: 1;
}

.resultPanel, .logsPanel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.resultBox {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 16px;
  background: var(--bg-panel);
  color: var(--text-main);
  flex: 1;
  overflow: auto;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  font-family: var(--font-main);
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.03);
}

.logControls {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  flex: 0 0 auto;
}

.logFilters {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.logFilter {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-sub);
  user-select: none;
}

.logMeta {
  font-size: 12px;
  color: var(--text-sub);
}

.logbox {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  background: #0f172a;
  color: #e2e8f0;
  flex: 1;
  min-height: 0;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
  font-family: var(--font-mono);
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.1);
}

.logLine {
  display: grid;
  grid-template-columns: 64px 74px 1fr;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
  align-items: start;
}

.logLine:last-child {
  border-bottom: none;
}

.logTime {
  font-size: 11px;
  color: #94a3b8;
}

.logTag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 18px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.2px;
  border: 1px solid rgba(226, 232, 240, 0.16);
  text-transform: lowercase;
}

.logTag.stdout {
  background: rgba(56, 189, 248, 0.15);
  border-color: rgba(56, 189, 248, 0.25);
  color: #7dd3fc;
}

.logTag.stderr {
  background: rgba(248, 113, 113, 0.15);
  border-color: rgba(248, 113, 113, 0.25);
  color: #fca5a5;
}

.logTag.system {
  background: rgba(148, 163, 184, 0.15);
  border-color: rgba(148, 163, 184, 0.25);
  color: #cbd5e1;
}

.logTag.assistant {
  background: rgba(34, 197, 94, 0.15);
  border-color: rgba(34, 197, 94, 0.25);
  color: #86efac;
}

.logMsg {
  white-space: pre-wrap;
  word-break: break-word;
}

.secretary {
  display: grid;
  gap: 20px;
  height: auto; /* Allow auto height now that we are sticky */
  grid-template-rows: auto 1fr auto;
}

.secretaryCards {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.secCard {
  border: none;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--bg-subtle) 0%, var(--color-primary-bg) 100%);
  padding: 16px;
  box-shadow: 0 2px 8px rgba(13, 148, 136, 0.1);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  transition: transform 0.2s, box-shadow 0.2s;
}

.secCard:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(13, 148, 136, 0.15);
}

.secK {
  font-size: 11px;
  color: var(--text-sub);
  text-transform: uppercase;
  font-weight: 700;
  letter-spacing: 0.05em;
}

.secV {
  font-size: 24px;
  font-weight: 800;
  color: var(--color-primary);
  margin-top: 6px;
}

.secSection {
  display: grid;
  gap: 10px;
}

.secSectionTitle {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-main);
}

.secRow {
  text-align: left;
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px;
  transition: all 0.2s;
  cursor: pointer;
}

.secRow:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: var(--color-primary-bg);
}

.briefing {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 14px;
  background: var(--bg-panel);
  font-size: 13px;
  color: var(--text-main);
  white-space: pre-wrap;
  overflow: auto;
  max-height: 200px;
  line-height: 1.6;
}

.secChat {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 16px;
  background: var(--bg-subtle);
}

.secChat summary {
  font-weight: 600;
  color: var(--color-primary);
  cursor: pointer;
}

.chatControls {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  margin: 12px 0;
  flex-wrap: wrap;
}

.chatControls label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
  color: var(--text-sub);
}

.chatControls select,
.chatControls input[type="number"] {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 8px 10px;
  background: var(--bg-panel);
  font-size: 13px;
}

.chatControls label.chatToggle {
  flex-direction: row;
  gap: 8px;
  align-items: center;
  padding-bottom: 8px;
}

.msgs {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px;
  overflow: auto;
  background: var(--bg-panel);
  max-height: 300px;
}

.msg {
  padding: 10px 14px;
  border-radius: var(--radius-md);
  margin-bottom: 10px;
  background: var(--bg-subtle);
  border: 1px solid transparent;
}

.msg.user {
  background: var(--color-primary-bg);
  color: var(--color-primary-hover);
}

.msg.streaming {
  border-style: dashed;
  opacity: 0.9;
}

.role {
  font-size: 11px;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 4px;
  font-weight: 700;
}

@media (max-width: 1300px) {
  .grid {
    grid-template-columns: 1fr;
    gap: 16px;
  }
  .secretaryCards {
    grid-template-columns: repeat(4, 1fr);
  }
  /* Disable sticky on mobile */
  .grid > section:first-child {
    position: static;
    max-height: none;
  }
}

@media (max-width: 720px) {
  .secOrb {
    right: 16px;
    bottom: 16px;
  }

  .secDrawer {
    top: 0;
    right: 0;
    bottom: 0;
    width: 100vw;
    border-radius: 0;
  }
}
</style>

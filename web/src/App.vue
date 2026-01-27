<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
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

const resumePrompt = ref<string>("");
const chatInput = ref<string>("");
const errorBanner = ref<string>("");
const chatBackend = ref<"auto" | "claude" | "codex">("auto");
const chatStreamEnabled = ref<boolean>(true);
const chatMaxSteps = ref<number>(8);
const chatStreamStatus = ref<string>("");
const chatStreamAnswer = ref<string>("");
const chatSending = ref<boolean>(false);

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

// Cockpit UI State
const sidebarExpanded = ref(false);
const secretaryOpen = ref(false);

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

async function onCreateTask() {
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
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

async function onSelectTask(id: string) {
  selectedTaskId.value = id;
  if (!logsByTask.value.has(id)) await loadLogs(id);
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
});

onBeforeUnmount(() => {
  if (es) es.close();
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
  if (!root) return sessionsAll.value;
  return sessionsAll.value.filter((s) => isWithinWorkspace(root, s.workdir));
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
        <button type="button" class="settingsBtn" @click="openAuthSettings">
          Settings
        </button>
      </div>
    </header>

    <div v-if="errorBanner" class="banner">{{ errorBanner }}</div>

    <div class="cockpit">
      <aside class="missionControl" :class="{ collapsed: !sidebarExpanded }">
        <div class="mcHeader">
          <div class="mcTitle">CCX Cockpit</div>
          <button class="mcToggle" @click="sidebarExpanded = !sidebarExpanded">
            {{ sidebarExpanded ? "◀" : "▶" }}
          </button>
        </div>
        <div class="mcBody">
          <div v-if="sidebarExpanded" class="form">
        <div class="form">
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
              v-model="newPrompt"
              rows="5"
              placeholder="Describe the task to run..."
            ></textarea>
          </label>
          <button
            class="primary"
            @click="onCreateTask"
            :disabled="!newPrompt.trim()"
          >
            Start
          </button>
        </div>

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

          </div> <!-- Close v-if=sidebarExpanded -->

          <div class="list">
            <div
              v-for="s in filteredSessions"
              :key="s.key"
              class="taskItem"
              :class="{ active: s.key === selectedSessionKey }"
              @click="onSelectTask(s.latest.id)"
              :title="s.latest.prompt"
            >
              <div class="taskIcon">
                 <span v-if="s.worker_type === 'claude-code'">🤖</span>
                 <span v-else-if="s.worker_type === 'exec'">💻</span>
                 <span v-else>🧠</span>
                 <div class="statusDot" :class="s.status"></div>
              </div>
              <div class="taskInfo">
                 <div class="taskTitle">{{ s.latest.prompt || s.session_id }}</div>
                 <div class="taskMeta">
                   <span>{{ (s.session_id || s.latest.id).slice(0, 8) }}</span>
                   <span>·</span>
                   <span :class="s.status">{{ s.status }}</span>
                 </div>
              </div>
             </div>
          </div>
        </div>
      </aside>

      <main class="viewport">
        <div class="viewHeader">
            <div class="hudStat">
               <span class="hudLabel">SESSION</span>
               <span class="hudValue">{{ selectedSession?.session_id?.slice(0,8) || "N/A" }}</span>
            </div>
             <div class="hudStat">
               <span class="hudLabel">STATUS</span>
               <span class="hudValue" :style="{ color: selectedSession?.status === 'running' ? '#22c55e' : '#94a3b8' }">
                 {{ selectedSession?.status || "IDLE" }}
               </span>
            </div>
            <div class="hudStat" v-if="selectedSession?.score">
               <span class="hudLabel">SCORE</span>
               <span class="hudValue">{{ selectedSession.score }}</span>
            </div>
             <div style="flex:1"></div>
             <!-- Actions will be injected here or below -->
        </div>

        <div v-if="!selectedSession" class="empty" style="padding: 40px; text-align:center; color: var(--text-sub);">
           No active session. Select a task from the sidebar or start a new one.
        </div>
        <div v-else class="viewBody">
          <!-- Hidden meta block refactored into HUD -->
          <div style="display:none">


          </div> <!-- Close hidden meta -->

          <div class="terminalArea">

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
          <div class="controlDeck" style="padding: 12px; border-top: 1px solid var(--border-color); background: rgba(0,0,0,0.2); display: flex; gap: 10px;">
             <button @click="onCancelTask" :disabled="selectedTask?.status !== 'running'" style="background: rgba(239, 68, 68, 0.1); color: #ef4444; border-color: rgba(239, 68, 68, 0.3);">
                Stop
             </button>
             <div style="flex:1"></div>
             <button @click="setWorkspace(selectedSession.workdir)">Focus Workdir</button>
          </div>
        </div>
      </main>

      <div class="secretaryOrb" @click="secretaryOpen = !secretaryOpen">
         <span style="font-size: 24px;">🤖</span>
      </div>
      <div class="secretaryPanel" :class="{ hidden: !secretaryOpen }">
         <div class="mcHeader">
            <div class="mcTitle">Secretary</div>
            <button class="iconBtn" @click="secretaryOpen = false">✕</button>
         </div>
         <div class="mcBody">
            <div class="secretary">
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
              @click="onSelectTask(s.latest.id)"
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

          <details class="secChat">
            <summary>Chat (optional)</summary>
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
                <div v-for="m in chat" :key="m.id" class="msg" :class="m.role">
                  <div class="role">{{ m.role }}</div>
                  <div class="content">{{ m.content }}</div>
                </div>
                <div v-if="chatStreamStatus || chatStreamAnswer" class="msg assistant streaming">
                  <div class="role">assistant</div>
                  <div class="content">
                    {{ chatStreamAnswer || chatStreamStatus }}
                  </div>
                </div>
              </div>
              <div class="input">
                <textarea
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
          </details>
        </div>
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
  --bg-app: #0f172a; /* Deep Space */
  --bg-panel: #1e293b; /* Slate 800 */
  --bg-subtle: #334155; /* Slate 700 */
  --color-primary: #38bdf8; /* Sky 400 */
  --color-primary-hover: #0ea5e9; /* Sky 500 */
  --color-primary-bg: rgba(56, 189, 248, 0.1);
  --text-main: #f8fafc; /* Slate 50 */
  --text-sub: #94a3b8; /* Slate 400 */
  --border-color: #334155;
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.3);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.3), 0 2px 4px -2px rgb(0 0 0 / 0.3);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.3), 0 4px 6px -4px rgb(0 0 0 / 0.3);
  --font-main: "Inter", ui-sans-serif, system-ui, -apple-system, sans-serif;
  --font-mono: "JetBrains Mono", ui-monospace, SFMono-Regular, monospace;
}

.page {
  font-family: var(--font-main);
  color: var(--text-main);
  background: var(--bg-app);
  height: 100vh;
  width: 100vw;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* --- Cockpit Layout --- */

.cockpit {
  display: flex;
  flex: 1;
  min-height: 0; /* Important for nested flex scroll */
  position: relative;
}

/* Mission Control (Sidebar) */
.missionControl {
  width: 320px;
  background: #111827;
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 20;
  flex-shrink: 0;
}

.missionControl.collapsed {
  width: 64px;
}

.mcHeader {
  height: 60px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-color);
  justify-content: space-between;
}

.mcTitle {
  font-weight: 800;
  font-size: 18px;
  background: linear-gradient(135deg, #38bdf8 0%, #818cf8 100%);
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  white-space: nowrap;
  overflow: hidden;
}

.mcToggle {
  color: var(--text-sub);
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 8px;
  border-radius: var(--radius-sm);
}
.mcToggle:hover {
  color: var(--text-main);
  background: rgba(255,255,255,0.05);
}

.mcBody {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* Viewport (Center Stage) */
.viewport {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: radial-gradient(circle at top right, #1e293b 0%, #0f172a 40%);
  position: relative;
}

.viewHeader {
  height: 60px;
  display: flex;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid var(--border-color);
  gap: 16px;
  background: rgba(15, 23, 42, 0.8);
  backdrop-filter: blur(8px);
}

.viewBody {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0; /* Full bleed */
  min-height: 0;
  position: relative;
}

/* Secretary (Floating Overlay) */
.secretaryOrb {
  position: absolute;
  bottom: 24px;
  right: 24px;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, #38bdf8 0%, #6366f1 100%);
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
  display: grid;
  place-items: center;
  cursor: pointer;
  z-index: 50;
  transition: transform 0.2s, box-shadow 0.2s;
  font-size: 24px;
  color: white;
}

.secretaryOrb:hover {
  transform: scale(1.05);
  box-shadow: 0 8px 20px rgba(99, 102, 241, 0.5);
}

.secretaryPanel {
  position: absolute;
  top: 60px; /* Below header */
  bottom: 24px;
  right: 24px;
  width: 400px;
  background: rgba(30, 41, 59, 0.95);
  backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
  box-shadow: -8px 0 32px rgba(0, 0, 0, 0.5);
  z-index: 40;
  display: flex;
  flex-direction: column;
  transform: translateX(0);
  transition: transform 0.3s ease, opacity 0.3s ease;
}

.secretaryPanel.hidden {
  transform: translateX(20px);
  opacity: 0;
  pointer-events: none;
}

/* --- Components --- */

/* Task Items in Sidebar */
.taskItem {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 0.2s;
  background: rgba(255,255,255,0.02);
}

.taskItem:hover {
  background: rgba(255,255,255,0.05);
}

.taskItem.active {
  background: rgba(56, 189, 248, 0.1);
  border-color: rgba(56, 189, 248, 0.3);
}

.taskIcon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: #334155;
  display: grid;
  place-items: center;
  font-size: 16px;
  flex-shrink: 0;
  position: relative;
}

.statusDot {
  position: absolute;
  bottom: -2px;
  right: -2px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid #111827;
}

.statusDot.running { background: #22c55e; box-shadow: 0 0 8px #22c55e; }
.statusDot.failed { background: #ef4444; }
.statusDot.succeeded { background: #3b82f6; }
.statusDot.queued { background: #eab308; }

.taskInfo {
  flex: 1;
  min-width: 0;
}

.taskTitle {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.taskMeta {
  font-size: 11px;
  color: var(--text-sub);
  display: flex;
  gap: 8px;
}

/* Collapsed Sidebar overrides */
.missionControl.collapsed .taskInfo,
.missionControl.collapsed .mcTitle {
  display: none;
}

.missionControl.collapsed .mcHeader {
  justify-content: center;
  padding: 0;
}

.missionControl.collapsed .taskItem {
  justify-content: center;
  padding: 10px 0;
}

/* HUD / Viewport Header */
.hudStat {
  display: flex;
  flex-direction: column;
}
.hudLabel {
  font-size: 10px;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.hudValue {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-main);
  font-family: var(--font-mono);
}

/* Terminal / Output Area */
.terminalArea {
  flex: 1;
  background: #0b0f19;
  border-top: 1px solid var(--border-color);
  padding: 20px;
  overflow-y: auto;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.6;
}

/* Modals & Inputs (Dark Theme Override) */
.modal, .settingsModal {
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  color: var(--text-main);
}
.modalHeader {
  background: rgba(0,0,0,0.2);
  border-bottom-color: var(--border-color);
}
.modalTitle { color: var(--text-main); }
.modalBody { color: var(--text-main); }
input, select, textarea {
  background: #0f172a;
  border-color: var(--border-color);
  color: var(--text-main);
}
input:focus, textarea:focus {
  border-color: var(--color-primary);
  background: #0f172a;
  box-shadow: 0 0 0 2px var(--color-primary-bg);
}
button {
  background: var(--bg-subtle);
  border-color: var(--border-color);
  color: var(--text-main);
}
button:hover:not(:disabled) {
  background: #475569;
  border-color: #64748b;
  color: white;
}
.logbox, .resultBox {
  background: #0b0f19 !important;
  color: #e2e8f0;
  border: none;
}

.logTag.stdout { background: rgba(56, 189, 248, 0.1); color: #7dd3fc; border: none; }
.logTag.stderr { background: rgba(248, 113, 113, 0.1); color: #fca5a5; border: none; }
.logTag.system { background: rgba(148, 163, 184, 0.1); color: #cbd5e1; border: none; }
.logTag.assistant { background: rgba(34, 197, 94, 0.1); color: #86efac; border: none; }
.logLine { border-bottom: 1px solid rgba(255,255,255,0.05); }

/* Secretary Chat in Dark Theme */
.chatMsg.assistant { background: rgba(255,255,255,0.05); }
.chatMsg.user { background: rgba(56, 189, 248, 0.15); border-left: 3px solid var(--color-primary); }

/* Scrollbars */
::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: #475569; border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: #64748b; }
</style>

/* Restored Form Styles */
.form { display: flex; flex-direction: column; gap: 12px; padding: 10px; }
.form label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-sub); }
.workdirRow { display: flex; gap: 8px; }
.authHint { background: rgba(255,100,0,0.1); color: #fb923c; padding: 8px; border-radius: 8px; font-size: 12px; }
.full { width: 100%; }


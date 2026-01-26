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
  updateAuth,
} from "./api";

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

const authInfo = ref<AuthInfo | null>(null);
const authStatus = computed<AuthStatus | null>(() => authInfo.value?.status ?? null);
const authSettingsOpen = ref(false);
const authSaving = ref(false);
const authSettingsError = ref("");
const authAnthropicBaseURL = ref("");
const authAnthropicApiKey = ref("");
const authAnthropicAuthToken = ref("");
const authAnthropicModel = ref("");
const authAnthropicSmallFastModel = ref("");
const authOpenAIApiKey = ref("");

const selectedTask = computed(() => tasks.value.get(selectedTaskId.value) ?? null);
const selectedLogs = computed(() => logsByTask.value.get(selectedTaskId.value) ?? []);

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
    return v.map((x) => (typeof x === "string" ? x.trim() : "")).filter(Boolean);
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

const pinnedWorkspaces = ref<string[]>(loadStringArray(LS_KEY_PINNED_WORKSPACES));
const workspaceFilter = ref<string>(loadString(LS_KEY_WORKSPACE_FILTER));

watch(pinnedWorkspaces, (v) => saveStringArray(LS_KEY_PINNED_WORKSPACES, v), { deep: true });
watch(workspaceFilter, (v) => {
  saveString(LS_KEY_WORKSPACE_FILTER, v);
  if (v.trim()) newWorkdir.value = v;
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
  tasks.value.set(task.id, task);
  if (!selectedTaskId.value) selectedTaskId.value = task.id;
}

function appendLog(entry: LogEntry) {
  const list = logsByTask.value.get(entry.task_id) ?? [];
  list.push(entry);
  logsByTask.value.set(entry.task_id, list);
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
  logsByTask.value.set(taskId, logs);
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
  errorBanner.value = "";
  try {
    await sendChat(msg);
    chatInput.value = "";
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
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
  pinnedWorkspaces.value = pinnedWorkspaces.value.filter((x) => normalizePathForCompare(x) !== key);
  if (normalizePathForCompare(workspaceFilter.value) === key) workspaceFilter.value = "";
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
        chat.value.push(evt.payload as ChatMessage);
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
    if (authAnthropicBaseURL.value.trim()) patch.anthropic_base_url = authAnthropicBaseURL.value.trim();
    if (authAnthropicApiKey.value.trim()) patch.anthropic_api_key = authAnthropicApiKey.value.trim();
    if (authAnthropicAuthToken.value.trim())
      patch.anthropic_auth_token = authAnthropicAuthToken.value.trim();
    if (authAnthropicModel.value.trim()) patch.anthropic_model = authAnthropicModel.value.trim();
    if (authAnthropicSmallFastModel.value.trim())
      patch.anthropic_small_fast_model = authAnthropicSmallFastModel.value.trim();
    if (authOpenAIApiKey.value.trim()) patch.openai_api_key = authOpenAIApiKey.value.trim();

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
  const pinned = new Set(pinnedWorkspaces.value.map((p) => normalizePathForCompare(p)));
  return recentWorkspaces.value.filter((p) => !pinned.has(normalizePathForCompare(p)));
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
    .filter((s) => s.status !== "succeeded" && (s.score > 0 || s.status === "failed" || s.status === "blocked"))
    .slice(0, 6);
});

const secretaryBriefing = computed(() => {
  const c = secretaryCounts.value;
  if (c.total === 0) return "当前还没有 session。";

  const lines: string[] = [];
  lines.push(`Session 总数：${c.total}`);
  lines.push(
    `running ${c.running} · blocked ${c.blocked} · failed ${c.failed} · interrupted ${c.interrupted} · queued ${c.queued} · succeeded ${c.succeeded}`
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
    const sid = s.session_id ? s.session_id.slice(0, 8) : s.latest.id.slice(0, 8);
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
          {{ systemInfo.os }}/{{ systemInfo.arch }} · {{ systemInfo.hostname }} · Go {{ systemInfo.go_version }}
        </div>
        <button type="button" class="settingsBtn" @click="openAuthSettings">Settings</button>
      </div>
    </header>

    <div v-if="errorBanner" class="banner">{{ errorBanner }}</div>

    <div class="grid">
      <section class="panel">
        <h2>Sessions</h2>
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
            <button type="button" @click="openAuthSettings">Auth Settings</button>
          </div>
          <label class="full">
            Prompt
            <textarea v-model="newPrompt" rows="5" placeholder="Describe the task to run..." />
          </label>
          <button class="primary" @click="onCreateTask" :disabled="!newPrompt.trim()">Start</button>
        </div>

        <div class="list">
          <div class="workspaceBar">
            <div class="workspaceLeft">
              <span class="workspaceTitle">Workspace</span>
              <select v-model="workspaceFilter">
                <option value="">All</option>
                <optgroup v-if="pinnedWorkspaces.length" label="Pinned">
                  <option v-for="p in pinnedWorkspaces" :key="'p-' + p" :value="p">{{ p }}</option>
                </optgroup>
                <optgroup v-if="recentWorkspacesUnpinned.length" label="Recent">
                  <option v-for="p in recentWorkspacesUnpinned" :key="'r-' + p" :value="p">{{ p }}</option>
                </optgroup>
              </select>
            </div>
            <button type="button" @click="setWorkspace(newWorkdir)" :disabled="!newWorkdir.trim()">
              Use Workdir
            </button>
            <button
              type="button"
              @click="pinWorkspace(workspaceFilter || newWorkdir)"
              :disabled="!(workspaceFilter || newWorkdir).trim()"
            >
              Pin
            </button>
            <button type="button" @click="clearWorkspace" :disabled="!workspaceFilter">All</button>
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
              <button type="button" class="pinnedX" @click="unpinWorkspace(p)" title="Unpin">✕</button>
            </div>
          </div>

          <div v-if="workspaceFilter" class="listMeta">
            Showing {{ filteredSessions.length }} / {{ sessionsAll.length }} sessions
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
          <div class="meta">
            <div>
              <span class="k">Session</span>
              <span class="mono">{{ selectedSession.session_id || "(pending)" }}</span>
            </div>
            <div><span class="k">Worker</span> {{ selectedSession.worker_type }}</div>
            <div><span class="k">Workdir</span> <span class="mono">{{ selectedSession.workdir }}</span></div>
            <div><span class="k">Status</span> {{ selectedSession.status }}</div>
            <div><span class="k">Score</span> {{ selectedSession.score }} (stderr {{ selectedSession.stderr_count }})</div>
            <div><span class="k">Runs</span> {{ selectedSession.runs.length }}</div>
            <div v-if="selectedSession.warning"><span class="k">Warning</span> {{ selectedSession.warning }}</div>
            <div v-if="selectedTask?.error"><span class="k">Last Err</span> {{ selectedTask.error }}</div>
          </div>

          <div class="actions">
            <button @click="onCancelTask" :disabled="selectedTask?.status !== 'running'">Cancel Run</button>
            <button type="button" @click="setWorkspace(selectedSession.workdir)">Focus Workdir</button>
          </div>

          <div class="resume">
            <label class="full">
              Resume Prompt
              <textarea v-model="resumePrompt" rows="3" placeholder="Continue with..." />
            </label>
            <button @click="onResumeTask" :disabled="!resumePrompt.trim() || !selectedSession.session_id">
              Resume Session
            </button>
          </div>

          <div class="runs">
            <div class="runsHeader">Runs</div>
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
            <div class="logsHeader">Logs</div>
            <pre class="logbox"><template v-for="l in selectedLogs" :key="l.id">[{{ l.stream }}] {{ l.message }}
</template></pre>
          </div>
        </div>
      </section>

      <section class="panel">
        <h2>Secretary</h2>
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
            <div v-if="needsAttentionSessions.length === 0" class="empty">暂无需要关注的 session</div>
            <button
              v-for="s in needsAttentionSessions"
              :key="s.key"
              type="button"
              class="secRow"
              @click="onSelectTask(s.latest.id)"
            >
              <div class="rowTop">
                <span class="mono">{{ (s.session_id || s.latest.id).slice(0, 8) }}</span>
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
              <div class="msgs">
                <div v-for="m in chat" :key="m.id" class="msg" :class="m.role">
                  <div class="role">{{ m.role }}</div>
                  <div class="content">{{ m.content }}</div>
                </div>
              </div>
              <div class="input">
                <textarea v-model="chatInput" rows="3" placeholder="Ask the secretary..." />
                <button class="primary" @click="onSendChat" :disabled="!chatInput.trim()">Send</button>
              </div>
            </div>
          </details>
        </div>
      </section>
    </div>

    <div v-if="authSettingsOpen" class="modalOverlay" @click.self="authSettingsOpen = false">
      <div class="modal settingsModal">
        <div class="modalHeader">
          <div class="modalTitle">Auth Settings</div>
          <button class="iconBtn" type="button" @click="authSettingsOpen = false">✕</button>
        </div>

        <div class="modalBody settingsBody">
          <div class="settingsMeta" v-if="authInfo?.storage_path">
            Storage: <span class="mono">{{ authInfo.storage_path }}</span>
          </div>

          <div v-if="authSettingsError" class="modalError">{{ authSettingsError }}</div>

          <div class="settingsSection">
            <div class="settingsSectionTitle">Claude Code</div>
            <div class="kv">
              <span class="k">ANTHROPIC_BASE_URL</span>
              <span class="mono"
                >{{ authStatus?.claude.base_url.effective }} {{ authStatus?.claude.base_url.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_API_KEY</span>
              <span class="mono">{{ authStatus?.claude.api_key.effective }} {{ authStatus?.claude.api_key.masked }}</span>
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_AUTH_TOKEN</span>
              <span class="mono"
                >{{ authStatus?.claude.auth_token.effective }} {{ authStatus?.claude.auth_token.masked }}</span
              >
            </div>
            <div class="kv">
              <span class="k">ANTHROPIC_MODEL</span>
              <span class="mono">{{ authStatus?.claude.model.effective }} {{ authStatus?.claude.model.masked }}</span>
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
                <input v-model="authAnthropicBaseURL" placeholder="https://..." autocomplete="off" />
                <button type="button" @click="clearStoredAuth('anthropic_base_url')" :disabled="authSaving">
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_API_KEY
              <div class="secretRow">
                <input v-model="authAnthropicApiKey" type="password" placeholder="Paste key…" autocomplete="off" />
                <button type="button" @click="clearStoredAuth('anthropic_api_key')" :disabled="authSaving">
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
                <button type="button" @click="clearStoredAuth('anthropic_auth_token')" :disabled="authSaving">
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_MODEL
              <div class="secretRow">
                <input v-model="authAnthropicModel" placeholder="model name…" autocomplete="off" />
                <button type="button" @click="clearStoredAuth('anthropic_model')" :disabled="authSaving">
                  Clear stored
                </button>
              </div>
            </label>
            <label class="full">
              Store ANTHROPIC_SMALL_FAST_MODEL
              <div class="secretRow">
                <input v-model="authAnthropicSmallFastModel" placeholder="model name…" autocomplete="off" />
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
              如果你使用 Claude Code 订阅登录模式，也可以在终端运行一次 <span class="mono">claude /login</span>。
            </div>
          </div>

          <div class="settingsSection">
            <div class="settingsSectionTitle">Codex</div>
            <div class="kv">
              <span class="k">OPENAI_API_KEY</span>
              <span class="mono">{{ authStatus?.codex.api_key.effective }} {{ authStatus?.codex.api_key.masked }}</span>
            </div>
            <label class="full">
              Store OPENAI_API_KEY
              <div class="secretRow">
                <input v-model="authOpenAIApiKey" type="password" placeholder="Paste key…" autocomplete="off" />
                <button type="button" @click="clearStoredAuth('openai_api_key')" :disabled="authSaving">
                  Clear stored
                </button>
              </div>
            </label>
          </div>
        </div>

        <div class="modalFooter">
          <button type="button" @click="authSettingsOpen = false">Close</button>
          <button type="button" class="primary" @click="saveAuthSettings" :disabled="authSaving">
            {{ authSaving ? "Saving..." : "Save" }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="dirPickerOpen" class="modalOverlay" @click.self="dirPickerOpen = false">
      <div class="modal">
        <div class="modalHeader">
          <div class="modalTitle">Select folder</div>
          <button class="iconBtn" type="button" @click="dirPickerOpen = false">✕</button>
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
            <button type="button" @click="dirParent && loadDir(dirParent)" :disabled="!dirParent">Up</button>
            <div class="path mono">{{ dirPath }}</div>
            <button type="button" class="primary" @click="selectDir(dirPath)" :disabled="!dirPath">Select</button>
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
.page {
  font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji",
    "Segoe UI Emoji";
  color: #111827;
  background: #f8fafc;
  min-height: 100vh;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
  background: white;
}
.headerRight {
  display: flex;
  align-items: center;
  gap: 10px;
}
.title {
  font-weight: 700;
  font-size: 18px;
}
.sub {
  color: #6b7280;
  font-size: 12px;
}
.settingsBtn {
  padding: 6px 10px;
}
.banner {
  margin: 10px 20px 0;
  background: #fee2e2;
  border: 1px solid #fecaca;
  color: #991b1b;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 13px;
}
.grid {
  display: grid;
  grid-template-columns: minmax(320px, 420px) minmax(560px, 1fr) minmax(340px, 480px);
  gap: 12px;
  padding: 12px 20px 20px;
  width: 100%;
  box-sizing: border-box;
}
.panel {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 12px;
  min-height: 200px;
  min-width: 0;
}
h2 {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 700;
}
.form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-bottom: 12px;
}
.form label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: #374151;
}
.form .full {
  grid-column: 1 / -1;
}
.authHint {
  grid-column: 1 / -1;
  display: flex;
  gap: 10px;
  align-items: center;
  background: #fff7ed;
  border: 1px solid #fed7aa;
  color: #9a3412;
  padding: 8px 10px;
  border-radius: 10px;
  font-size: 12px;
}
.authHint .text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
input,
select,
textarea {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 8px;
  font-size: 13px;
  outline: none;
}
textarea {
  resize: vertical;
}
button {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 8px 10px;
  background: #f9fafb;
  cursor: pointer;
}
.workdirRow {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 8px;
}
.modalOverlay {
  position: fixed;
  inset: 0;
  background: rgba(17, 24, 39, 0.55);
  display: grid;
  place-items: center;
  padding: 18px;
  z-index: 999;
}
.modal {
  width: min(860px, 95vw);
  height: min(560px, 90vh);
  background: white;
  border-radius: 14px;
  border: 1px solid #e5e7eb;
  display: grid;
  grid-template-rows: auto 1fr auto;
  overflow: hidden;
}
.modalHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 12px;
  border-bottom: 1px solid #e5e7eb;
}
.modalTitle {
  font-weight: 700;
  font-size: 14px;
}
.iconBtn {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 6px 10px;
  background: #f9fafb;
}
.modalBody {
  padding: 12px;
  overflow: auto;
}
.dirModalBody {
  display: grid;
  grid-template-rows: auto auto auto auto 1fr;
  gap: 10px;
  overflow: hidden;
}
.settingsModal {
  width: min(760px, 95vw);
  height: min(560px, 90vh);
}
.settingsBody {
  display: grid;
  gap: 12px;
  overflow: auto;
}
.settingsMeta {
  color: #6b7280;
  font-size: 12px;
}
.settingsSection {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px;
  display: grid;
  gap: 8px;
}
.settingsSectionTitle {
  font-weight: 700;
  font-size: 13px;
}
.kv {
  display: grid;
  grid-template-columns: 180px 1fr;
  gap: 8px;
  align-items: center;
  font-size: 12px;
  color: #374151;
}
.settingsHelp {
  color: #6b7280;
  font-size: 12px;
}
.secretRow {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 8px;
}
.roots {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.rootBtn {
  font-size: 12px;
  padding: 6px 10px;
  border-radius: 999px;
}
.pathRow {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 8px;
  align-items: center;
}
.path {
  padding: 8px 10px;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  background: #f9fafb;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.filterRow {
  display: flex;
  gap: 8px;
  align-items: center;
}
.loading {
  font-size: 12px;
  color: #6b7280;
}
.dirList {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  overflow: auto;
  padding: 6px;
  background: #fff;
}
.dirItem {
  width: 100%;
  display: grid;
  grid-template-columns: 22px 1fr;
  gap: 8px;
  align-items: center;
  text-align: left;
  border: 1px solid transparent;
  background: transparent;
  padding: 8px 10px;
  border-radius: 10px;
}
.dirItem:hover {
  border-color: #e5e7eb;
  background: #f9fafb;
}
.dirItem .name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.modalError {
  background: #fee2e2;
  border: 1px solid #fecaca;
  color: #991b1b;
  padding: 8px 10px;
  border-radius: 10px;
  font-size: 12px;
}
.modalFooter {
  padding: 12px;
  border-top: 1px solid #e5e7eb;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
button.primary {
  background: #111827;
  color: white;
  border-color: #111827;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.workspaceBar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 0;
}
.workspaceLeft {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}
.workspaceTitle {
  font-size: 12px;
  color: #6b7280;
  white-space: nowrap;
}
.workspaceBar select {
  flex: 1;
  min-width: 0;
}
.pinnedWorkspaces {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.pinnedItem {
  display: flex;
  align-items: stretch;
  border: 1px solid #e5e7eb;
  border-radius: 999px;
  overflow: hidden;
  background: #f9fafb;
}
.pinnedBtn {
  border: none;
  background: transparent;
  padding: 6px 10px;
  cursor: pointer;
  max-width: 260px;
  text-align: left;
}
.pinnedBtn.active {
  background: #111827;
  color: white;
}
.pinnedBtn .mono {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pinnedX {
  border: none;
  border-left: 1px solid #e5e7eb;
  background: transparent;
  padding: 6px 8px;
  cursor: pointer;
}
.listMeta {
  font-size: 12px;
  color: #6b7280;
  margin-top: -2px;
}
.row {
  text-align: left;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px;
}
.row.active {
  border-color: #111827;
}
.rowTop {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}
.rowMid {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #6b7280;
  font-size: 12px;
  margin-bottom: 6px;
}
.rowPath {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rowPath.mono {
  overflow-wrap: normal;
}
.rowBottom {
  font-size: 12px;
  color: #111827;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  overflow-wrap: anywhere;
}
.pill {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid #e5e7eb;
}
.pill.kind {
  background: #f3f4f6;
}
.pill.running {
  background: #dbeafe;
  border-color: #bfdbfe;
}
.pill.succeeded {
  background: #dcfce7;
  border-color: #bbf7d0;
}
.pill.failed {
  background: #fee2e2;
  border-color: #fecaca;
}
.pill.canceled,
.pill.interrupted,
.pill.queued,
.pill.blocked {
  background: #f3f4f6;
}
.score {
  font-weight: 600;
}
.warn {
  color: #b45309;
}
.detail .meta {
  display: grid;
  gap: 6px;
  font-size: 12px;
  color: #374151;
}
.detail .k {
  display: inline-block;
  width: 64px;
  color: #6b7280;
}
.actions {
  margin: 10px 0;
  display: flex;
  gap: 8px;
}
.resume {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
}
.runs {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
}
.runsHeader {
  font-size: 12px;
  color: #6b7280;
}
.runList {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  overflow: auto;
  padding: 6px;
  background: #fff;
  max-height: 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.runRow {
  text-align: left;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px;
}
.runRow.active {
  border-color: #111827;
}
.runTop {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}
.runMid {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #6b7280;
  font-size: 12px;
  margin-bottom: 6px;
}
.runBottom {
  font-size: 12px;
  color: #111827;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.logsHeader {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 6px;
}
.logbox {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 10px;
  background: #0b1020;
  color: #d1d5db;
  height: 360px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.4;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.empty {
  color: #6b7280;
  font-size: 13px;
}
.secretary {
  display: grid;
  gap: 12px;
  height: calc(100vh - 120px);
}
.secretaryCards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}
.secCard {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #f9fafb;
  padding: 10px;
}
.secK {
  font-size: 12px;
  color: #6b7280;
}
.secV {
  font-size: 18px;
  font-weight: 800;
  margin-top: 2px;
}
.secSection {
  display: grid;
  gap: 8px;
}
.secSectionTitle {
  font-size: 12px;
  color: #6b7280;
}
.secRow {
  text-align: left;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px;
}
.secRow:hover {
  border-color: #d1d5db;
  background: #f9fafb;
}
.briefing {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 10px;
  background: #f9fafb;
  font-size: 12px;
  color: #111827;
  white-space: pre-wrap;
  overflow: auto;
  max-height: 240px;
}
.secChat {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px;
  background: #fff;
}
.secChat summary {
  cursor: pointer;
  font-size: 12px;
  color: #374151;
}
.secChat .chat {
  height: auto;
  margin-top: 10px;
}
.secChat .msgs {
  max-height: 320px;
}
.chat {
  display: grid;
  grid-template-rows: 1fr auto;
  gap: 8px;
  height: calc(100vh - 120px);
}
.msgs {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 10px;
  overflow: auto;
  background: #f9fafb;
}
.msg {
  display: grid;
  gap: 4px;
  padding: 8px;
  border-radius: 10px;
  margin-bottom: 8px;
  background: white;
  border: 1px solid #e5e7eb;
}
.msg.assistant {
  border-color: #bfdbfe;
}
.role {
  font-size: 11px;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.content {
  font-size: 13px;
  white-space: pre-wrap;
}
.input {
  display: grid;
  gap: 8px;
}
@media (max-width: 1100px) {
  .grid {
    grid-template-columns: 1fr;
  }
  .secretary {
    height: auto;
  }
  .chat {
    height: auto;
  }
  .logbox {
    height: 260px;
  }
}
</style>

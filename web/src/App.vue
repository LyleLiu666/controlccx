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

watch(pinnedWorkspaces, (v) => saveStringArray(LS_KEY_PINNED_WORKSPACES, v), {
  deep: true,
});
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
              <span class="mono">{{
                selectedSession.session_id || "(pending)"
              }}</span>
            </div>
            <div>
              <span class="k">Worker</span> {{ selectedSession.worker_type }}
            </div>
            <div>
              <span class="k">Workdir</span>
              <span class="mono">{{ selectedSession.workdir }}</span>
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
            <div v-if="selectedSession.warning">
              <span class="k">Warning</span> {{ selectedSession.warning }}
            </div>
            <div v-if="selectedTask?.error">
              <span class="k">Last Err</span> {{ selectedTask.error }}
            </div>
          </div>

          <div class="actions">
            <button
              @click="onCancelTask"
              :disabled="selectedTask?.status !== 'running'"
            >
              Cancel Run
            </button>
            <button
              type="button"
              @click="setWorkspace(selectedSession.workdir)"
            >
              Focus Workdir
            </button>
          </div>

          <div class="resume">
            <label class="full">
              Resume Prompt
              <textarea
                v-model="resumePrompt"
                rows="3"
                placeholder="Continue with..."
              ></textarea>
            </label>
            <button
              @click="onResumeTask"
              :disabled="!resumePrompt.trim() || !selectedSession.session_id"
            >
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
            <pre
              class="logbox"
            ><template v-for="l in selectedLogs" :key="l.id">[{{ l.stream }}] {{ l.message }}
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
              <div class="msgs">
                <div v-for="m in chat" :key="m.id" class="msg" :class="m.role">
                  <div class="role">{{ m.role }}</div>
                  <div class="content">{{ m.content }}</div>
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
                  :disabled="!chatInput.trim()"
                >
                  Send
                </button>
              </div>
            </div>
          </details>
        </div>
      </section>
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
:root {
  --bg-app: #f1f5f9; /* Slate 100 - slightly darker than before for contrast */
  --bg-panel: #ffffff;
  --bg-subtle: #f8fafc;
  --color-primary: #0d9488; /* Teal 600 */
  --color-primary-hover: #0f766e; /* Teal 700 */
  --color-primary-bg: #ccfbf1; /* Teal 100 */
  --text-main: #334155; /* Slate 700 */
  --text-sub: #64748b; /* Slate 500 */
  --border-color: #e2e8f0;
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.06), 0 2px 4px -2px rgb(0 0 0 / 0.06);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.05), 0 4px 6px -4px rgb(0 0 0 / 0.05);
  --font-main: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}

.page {
  font-family: var(--font-main);
  color: var(--text-main);
  background: var(--bg-app);
  min-height: 100vh;
  box-sizing: border-box;
  padding-bottom: 20px;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 32px;
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(12px);
  position: sticky;
  top: 0;
  z-index: 50;
  border-bottom: 1px solid rgba(255, 255, 255, 0.5);
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
  grid-template-columns: minmax(340px, 380px) minmax(500px, 1fr) minmax(300px, 340px);
  gap: 24px;
  padding: 0 32px;
  width: 100%;
  box-sizing: border-box;
  max-width: 1920px;
  margin: 0 auto;
  align-items: start; /* Important for sticky to work */
}

.panel {
  background: var(--bg-panel);
  border: 1px solid white;
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
.grid > section:first-child,
.grid > section:last-child {
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
  color: var(--text-main);
  background: #f8fafc;
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  gap: 8px;
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

.authHint .text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

input,
select,
textarea {
  border: 1px solid var(--border-color);
  background: #f8fafc;
  border-radius: var(--radius-md);
  padding: 10px 12px;
  font-size: 14px;
  outline: none;
  transition: all 0.2s;
  color: var(--text-main);
  font-family: var(--font-main);
  width: 100%;
  box-sizing: border-box;
}

input:focus,
select:focus,
textarea:focus {
  border-color: var(--color-primary);
  background: #fff;
  box-shadow: 0 0 0 3px var(--color-primary-bg);
}

textarea {
  resize: vertical;
  line-height: 1.5;
}

button {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 8px 14px;
  background: white;
  color: var(--text-main);
  font-weight: 500;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

button:hover:not(:disabled) {
  background: #f8fafc;
  border-color: #cbd5e1;
  color: var(--color-primary);
}

button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  background: #f1f5f9;
}

button.primary {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
  box-shadow: 0 2px 4px rgba(13, 148, 136, 0.2);
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

.modalOverlay {
  position: fixed;
  inset: 0;
  background: rgba(240, 253, 250, 0.6);
  backdrop-filter: blur(4px);
  display: grid;
  place-items: center;
  padding: 24px;
  z-index: 999;
}

.modal, .settingsModal {
  background: white;
  border-radius: 24px;
  border: 1px solid #fff;
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
  background: #f8fafc;
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
  background: #e2e8f0;
  color: var(--text-main);
}

.modalBody, .settingsBody, .dirModalBody {
  padding: 20px;
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
  background: #fcfcfc;
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
  background: white;
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
  background: #f8fafc;
  color: var(--text-sub);
  font-family: var(--font-mono);
  font-size: 13px;
}

.dirList {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
  padding: 8px;
  background: #fff;
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
  background: white;
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
  background: white;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: var(--shadow-sm);
  cursor: pointer;
  position: relative;
  overflow: hidden;
}

.row:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 12px -3px rgba(0,0,0,0.05);
  border-color: var(--color-primary-bg);
}

.row.active {
  border-color: var(--color-primary);
  background: #f0fdfa;
  box-shadow: 0 0 0 2px var(--color-primary-bg);
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
  background: #f1f5f9;
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
  background: #e2e8f0;
  color: var(--text-sub);
}

.pill.running {
  background: #dbeafe;
  color: #1e40af;
}

.pill.succeeded {
  background: #d1fae5;
  color: #065f46;
}

.pill.failed {
  background: #fee2e2;
  color: #991b1b;
}

.pill.canceled,
.pill.interrupted,
.pill.queued,
.pill.blocked {
  background: #f1f5f9;
  color: #64748b;
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

.detail .meta {
  display: grid;
  gap: 8px;
  font-size: 13px;
  background: white;
  padding: 16px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  margin-bottom: 16px;
  box-shadow: var(--shadow-sm);
}

.detail .k {
  display: inline-block;
  width: 70px;
  color: var(--text-sub);
  font-weight: 600;
}

.actions {
  margin-bottom: 16px;
  display: flex;
  gap: 10px;
}

.resume {
  display: grid;
  gap: 10px;
  margin-bottom: 16px;
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
}

.runList {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
  padding: 8px;
  background: #f8fafc;
  max-height: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.03);
}

.runRow {
  background: white;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 10px;
  transition: all 0.2s;
  cursor: pointer;
}

.runRow:hover {
  border-color: var(--color-primary);
}
.runRow.active {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-bg);
}

.logs {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 400px; /* Ensure reasonable height */
}

.logsHeader {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-main);
  margin-bottom: 8px;
}

.logbox {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 14px;
  background: #1e293b; /* Dark Slate Blue for logs */
  color: #e2e8f0;
  flex: 1;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  font-family: var(--font-mono);
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.1);
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
  border: 1px solid white;
  border-radius: var(--radius-md);
  background: #f8fafc;
  padding: 16px;
  box-shadow: var(--shadow-sm);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
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
  background: white;
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
  background: white;
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
  background: #f8fafc;
}

.secChat summary {
  font-weight: 600;
  color: var(--color-primary);
  cursor: pointer;
}

.msgs {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px;
  overflow: auto;
  background: white;
  max-height: 300px;
}

.msg {
  padding: 10px 14px;
  border-radius: var(--radius-md);
  margin-bottom: 10px;
  background: #f8fafc;
  border: 1px solid transparent;
}

.msg.user {
  background: var(--color-primary-bg);
  color: #0f766e;
}

.role {
  font-size: 11px;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 4px;
  font-weight: 700;
}

@media (max-width: 1100px) {
  .grid {
    grid-template-columns: 1fr;
    gap: 16px;
  }
  .secretaryCards {
    grid-template-columns: repeat(4, 1fr);
  }
  /* Disable sticky on mobile */
  .grid > section:first-child,
  .grid > section:last-child {
    position: static;
    max-height: none;
  }
}
</style>

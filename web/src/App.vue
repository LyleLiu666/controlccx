<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { ChatMessage, LogEntry, ServerEvent, SystemInfo, Task, WorkerType } from "./types";
import {
  cancelTask,
  createTask,
  fetchChat,
  fetchLogs,
  fetchSystemInfo,
  fetchTasks,
  resumeTask,
  sendChat,
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

const selectedTask = computed(() => tasks.value.get(selectedTaskId.value) ?? null);
const selectedLogs = computed(() => logsByTask.value.get(selectedTaskId.value) ?? []);

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
  const t = selectedTask.value;
  if (!t) return;
  if (!t.session_id) {
    errorBanner.value = "该任务没有 session_id，无法 resume。";
    return;
  }
  errorBanner.value = "";
  try {
    const nt = await resumeTask(t.id, resumePrompt.value);
    upsertTask(nt);
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

onMounted(async () => {
  await refresh();
  if (selectedTaskId.value) await loadLogs(selectedTaskId.value);
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
</script>

<template>
  <div class="page">
    <header class="header">
      <div class="title">ControlCCX</div>
      <div class="sub" v-if="systemInfo">
        {{ systemInfo.os }}/{{ systemInfo.arch }} · {{ systemInfo.hostname }} · Go {{ systemInfo.go_version }}
      </div>
    </header>

    <div v-if="errorBanner" class="banner">{{ errorBanner }}</div>

    <div class="grid">
      <section class="panel">
        <h2>Tasks</h2>
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
            <input v-model="newWorkdir" placeholder="." />
          </label>
          <label class="full">
            Prompt
            <textarea v-model="newPrompt" rows="5" placeholder="Describe the task to run..." />
          </label>
          <button class="primary" @click="onCreateTask" :disabled="!newPrompt.trim()">Start</button>
        </div>

        <div class="list">
          <button
            v-for="t in sortedTasks"
            :key="t.id"
            class="row"
            :class="{ active: t.id === selectedTaskId }"
            @click="onSelectTask(t.id)"
          >
            <div class="rowTop">
              <span class="mono">{{ t.id.slice(0, 8) }}</span>
              <span class="pill" :class="t.status">{{ t.status }}</span>
            </div>
            <div class="rowMid">
              <span class="pill kind">{{ t.worker_type }}</span>
              <span class="score">score {{ t.score }}</span>
              <span v-if="t.warning" class="warn">⚠</span>
            </div>
            <div class="rowBottom">{{ t.prompt }}</div>
          </button>
        </div>
      </section>

      <section class="panel">
        <h2>Task Detail</h2>
        <div v-if="!selectedTask" class="empty">Select a task</div>
        <div v-else class="detail">
          <div class="meta">
            <div><span class="k">ID</span> <span class="mono">{{ selectedTask.id }}</span></div>
            <div><span class="k">Worker</span> {{ selectedTask.worker_type }} ({{ selectedTask.mode }})</div>
            <div><span class="k">Status</span> {{ selectedTask.status }}</div>
            <div><span class="k">Score</span> {{ selectedTask.score }} (stderr {{ selectedTask.stderr_count }})</div>
            <div v-if="selectedTask.session_id"><span class="k">Session</span> <span class="mono">{{ selectedTask.session_id }}</span></div>
            <div v-if="selectedTask.warning"><span class="k">Warning</span> {{ selectedTask.warning }}</div>
            <div v-if="selectedTask.error"><span class="k">Error</span> {{ selectedTask.error }}</div>
          </div>

          <div class="actions">
            <button @click="onCancelTask" :disabled="selectedTask.status !== 'running'">Cancel</button>
          </div>

          <div class="resume">
            <label class="full">
              Resume Prompt
              <textarea v-model="resumePrompt" rows="3" placeholder="Continue with..." />
            </label>
            <button @click="onResumeTask" :disabled="!resumePrompt.trim() || !selectedTask.session_id">Resume</button>
          </div>

          <div class="logs">
            <div class="logsHeader">Logs</div>
            <pre class="logbox"><template v-for="l in selectedLogs" :key="l.id">[{{ l.stream }}] {{ l.message }}
</template></pre>
          </div>
        </div>
      </section>

      <section class="panel">
        <h2>Observer Chat</h2>
        <div class="chat">
          <div class="msgs">
            <div v-for="m in chat" :key="m.id" class="msg" :class="m.role">
              <div class="role">{{ m.role }}</div>
              <div class="content">{{ m.content }}</div>
            </div>
          </div>
          <div class="input">
            <textarea v-model="chatInput" rows="3" placeholder="Ask the observer..." />
            <button class="primary" @click="onSendChat" :disabled="!chatInput.trim()">Send</button>
          </div>
        </div>
      </section>
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
  align-items: baseline;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
  background: white;
}
.title {
  font-weight: 700;
  font-size: 18px;
}
.sub {
  color: #6b7280;
  font-size: 12px;
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
  grid-template-columns: 1.1fr 1.6fr 1.2fr;
  gap: 12px;
  padding: 12px 20px 20px;
}
.panel {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 12px;
  min-height: 200px;
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
}
.empty {
  color: #6b7280;
  font-size: 13px;
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
  .chat {
    height: auto;
  }
  .logbox {
    height: 260px;
  }
}
</style>

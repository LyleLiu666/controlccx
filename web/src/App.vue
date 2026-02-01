<script setup lang="ts">
import MarkdownIt from "markdown-it";
import mermaid from "mermaid";
import hljs from "highlight.js/lib/common";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type {
  AcceptanceState,
  AuthInfo,
  AuthPatch,
  AuthStatus,
  ChatMessage,
  FSEntry,
  FSListEntry,
  FSRoot,
  LogEntry,
  SessionWorkspace,
  Tool,
  ToolDriver,
  ToolsListResponse,
  SystemInfo,
  Task,
  WorkerType,
} from "./types";
import {
  cancelTask,
  createTask,
  deleteSession,
  fetchAuthInfo,
  fetchChat,
  fetchFSEntries,
  fetchFSList,
  fetchFSRead,
  fetchFSRoots,
  fetchLogs,
  fetchTools,
  fetchSystemInfo,
  fsDelete,
  fsMkdir,
  fsWrite,
  deleteTool,
  renameSession,
  resumeTaskWithOptions,
  rehydrateTaskWithOptions,
  sendChat,
  upsertTool,
  updateAuth,
  fetchAcceptance,
} from "./api";
import { appendChatMessageUnique, sendChatAndReload } from "./chatOps";
import {
  attentionAutopilotIsNoConversationFound,
  attentionAutopilotMarkSeen,
  attentionAutopilotSeenAtMs,
  attentionAutopilotShouldAttempt,
  attentionAutopilotStopForSession,
} from "./attentionAutopilot";
import { shouldOfferRehydrateForTask, type ResumeOrigin } from "./rehydrate";
import { computePopupPosition } from "./menuPosition";
import { prettifyLogMessage } from "./logPretty";
import { deriveRunActivity } from "./runActivity";
import type { RunSafetyPayload, TaskIntent } from "./runSafety";
import {
  buildRunSafetyPayload,
  effectiveSafetyPresetForTask,
  isHighRiskPreset,
  normalizeSafetyPreset,
  normalizeTaskIntent,
  safetyPresetsForDriver,
} from "./runSafety";
import SkillsPanel from "./components/SkillsPanel.vue";
import SkillsVersionsPanel from "./components/SkillsVersionsPanel.vue";
import SkillsGovernancePanel from "./components/SkillsGovernancePanel.vue";
import SecretaryDrawer from "./components/SecretaryDrawer.vue";
import LiveDrawer from "./components/LiveDrawer.vue";
import FilesModal from "./components/FilesModal.vue";
import AuthSettingsModal from "./components/AuthSettingsModal.vue";
import ToolsSettingsModal from "./components/ToolsSettingsModal.vue";
import { useSkills } from "./composables/useSkills";
import { useSkillVersions } from "./composables/useSkillVersions";
import { useSecretaryChat } from "./composables/useSecretaryChat";
import { useTaskWorkspace } from "./composables/useTaskWorkspace";
import { useTasks } from "./composables/useTasks";
import { useLiveFeed } from "./composables/useLiveFeed";

const systemInfo = ref<SystemInfo | null>(null);

const newWorkerType = ref<WorkerType>("claude-code");
const newWorkdir = ref<string>(".");
const newPrompt = ref<string>("");
const newRunOpen = ref(false);
const newRunPromptEl = ref<HTMLTextAreaElement | null>(null);

const newRunSafetyOverride = ref(false);
const newRunHighRiskOptIn = ref(false);

const resumePrompt = ref<string>("");
const resumeExpanded = ref(true);
const resumeSafetyOverride = ref(false);
const resumeHighRiskOptIn = ref(false);
const errorBanner = ref<string>("");

const highRiskConfirmOpen = ref(false);
const highRiskConfirmTitle = ref("");
const highRiskConfirmMessage = ref("");
const highRiskConfirmDetail = ref("");
const highRiskConfirmConfirmLabel = ref("Continue");
const highRiskConfirmBusy = ref(false);
let highRiskConfirmResolve: ((ok: boolean) => void) | null = null;

function highRiskPresetSummary(driver: ToolDriver, preset: string): string {
  const d = String(driver ?? "").trim();
  const p = String(preset ?? "").trim();
  if (d === "codex" && p === "unsafe") {
    return "Codex: --dangerously-bypass-approvals-and-sandbox (no sandbox)";
  }
  if (d === "codex" && p === "danger-full-access") {
    return "Codex: --sandbox danger-full-access (can access outside workspace)";
  }
  if (d === "claude-code" && p === "unsafe") {
    return "Claude Code: --dangerously-skip-permissions";
  }
  if (d && p) return `${d}: ${p}`;
  if (d) return d;
  return "";
}

function requestHighRiskConfirm(opts: {
  title: string;
  message: string;
  detail?: string;
  confirmLabel?: string;
}): Promise<boolean> {
  if (highRiskConfirmOpen.value) return Promise.resolve(false);
  highRiskConfirmTitle.value = String(opts.title ?? "").trim() || "Confirm";
  highRiskConfirmMessage.value = String(opts.message ?? "").trim();
  highRiskConfirmDetail.value = String(opts.detail ?? "").trim();
  highRiskConfirmConfirmLabel.value = String(opts.confirmLabel ?? "").trim() || "Continue";
  highRiskConfirmBusy.value = false;
  highRiskConfirmOpen.value = true;
  return new Promise((resolve) => {
    highRiskConfirmResolve = resolve;
  });
}

function closeHighRiskConfirm(ok: boolean) {
  if (!highRiskConfirmOpen.value) return;
  highRiskConfirmOpen.value = false;
  highRiskConfirmBusy.value = false;
  const resolve = highRiskConfirmResolve;
  highRiskConfirmResolve = null;
  resolve?.(ok);
}

async function confirmHighRiskConfirm() {
  if (highRiskConfirmBusy.value) return;
  highRiskConfirmBusy.value = true;
  closeHighRiskConfirm(true);
}

function cancelHighRiskConfirm() {
  closeHighRiskConfirm(false);
}
const {
  chat,
  chatInput,
  chatBackend,
  chatStreamEnabled,
  chatMaxSteps,
  chatStreamStatus,
  chatStreamAnswer,
  chatSending,
  sendChatMessage,
} = useSecretaryChat({
  onError: (message: string) => {
    errorBanner.value = message;
  },
});

const theme = ref<"light" | "dark">("light");
const headerMoreEl = ref<HTMLDetailsElement | null>(null);

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

const toolsLoading = ref(false);
const toolsError = ref("");
const toolsList = ref<Tool[]>([]);
const toolsSettingsOpen = ref(false);
const toolsSaving = ref(false);
const toolsSelectedID = ref<string>("");
const toolEditID = ref("");
const toolEditDriver = ref<ToolDriver>("claude-code");
const toolEditCommand = ref("");
const toolEditArgs = ref("");
const toolEditEnv = ref("");

const {
  skillsOpen,
  skillsLoading,
  skillsError,
  skillsFilter,
  skillsLimit,
  skillsData,
  skillsRangeLabel,
  skillsCanPrev,
  skillsCanNext,
  skillsActionBusy,
  openSkills,
  refreshSkills,
  skillsPrevPage,
  skillsNextPage,
  skillsKey,
  onSkillsToggle,
  summarizeSkillTarget,
  skillBadgeClass,
} = useSkills();

const {
  skillsVersionsLoading,
  skillsVersionsError,
  skillsVersionsData,
  skillsVersionNewId,
  skillsVersionNewNote,
  skillsVersionsCreating,
  skillsVersionsDeleting,
  refreshSkillVersions,
  createSkillVersionFromForm,
  deleteSkillVersionByID,
} = useSkillVersions();

const outputTab = ref<"result" | "logs" | "trace">("result");
const resultPreviewTab = ref<"markdown" | "raw" | "html">("markdown");
const logPreviewTab = ref<"pretty" | "raw">("pretty");
const logShowAssistant = ref(true);
const logShowStdout = ref(true);
const logShowStderr = ref(true);
const logShowSystem = ref(true);
const logSearch = ref("");
const sessionSearch = ref("");
const sessionsLimit = ref(40);
const sessionsShowDeleted = ref(false);
const homeRunBusy = ref(false);

const filePreviewOpen = ref(false);
const filePreviewRawPath = ref("");
const filePreviewBase = ref("");
const filePreviewResolvedPath = ref("");
const filePreviewSize = ref<number>(0);
const filePreviewTruncated = ref(false);
const filePreviewContent = ref("");
const filePreviewLoading = ref(false);
const filePreviewError = ref("");
const filePreviewBoxEl = ref<HTMLDivElement | null>(null);
const filePreviewTab = ref<"preview" | "raw" | "html">("preview");

const {
  tasks,
  selectedTaskId,
  selectedTask,
  selectedLogs,
  logsByTask,
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
  connectEvents,
  reconnectEvents,
} = useTasks({
  showDeleted: sessionsShowDeleted,
  autoSelectFirst: false,
  onTaskUpsert: (prev, next) => {
    // Fire-and-forget; avoid blocking SSE handling.
    void maybeTriggerDeliveryForeman(prev, next);
    maybeTriggerAttentionAutopilot(prev, next);
    void maybePromptWorkspaceMerge(prev, next);
    void maybePromptRehydrate(prev, next);
  },
  onChatMessage: (m) => {
    chat.value = appendChatMessageUnique(chat.value, m);
  },
});

const {
  workspace: runWorkspace,
  loading: runWorkspaceLoading,
  error: runWorkspaceError,
  conflict: runWorkspaceConflict,
  refresh: refreshRunWorkspace,
  merge: mergeRunWorkspace,
  discard: discardRunWorkspace,
} = useTaskWorkspace(selectedTaskId);

function conflictSummary(c: any): string {
  if (!c) return "";
  const msg = String(c.message ?? "").trim() || "conflict";
  const list = Array.isArray(c.conflicts) ? c.conflicts.filter((x: any) => typeof x === "string" && x.trim()) : [];
  if (!list.length) return msg;
  const head = list.slice(0, 4).join(", ");
  const tail = list.length > 4 ? ` +${list.length - 4} more` : "";
  return `${msg}: ${head}${tail}`;
}

async function onMergeRunWorkspace() {
  const ws = runWorkspace.value;
  if (!ws || ws.status !== "active") return;
  if (!confirm("Merge isolated workspace changes back into your base workdir/branch?")) return;
  await mergeRunWorkspace();
}

async function onDiscardRunWorkspace() {
  const ws = runWorkspace.value;
  if (!ws || ws.status !== "active") return;
  if (!confirm("Discard isolated workspace changes? (This deletes the run workspace folder.)")) return;
  await discardRunWorkspace();
}

const selectedRunInstruction = computed(() => {
  const t = selectedTask.value;
  if (!t) return "";
  const mode = t.mode === "resume" ? "Resume" : "New";
  const p = promptSummary(t.prompt);
  return p ? `${mode} · ${p}` : mode;
});
const selectedRunActivity = computed(() => {
  const t = selectedTask.value;
  if (!t) return null;
  if (!(t.status === "running" || t.status === "queued")) return null;
  return deriveRunActivity(selectedLogs.value);
});

const acceptanceState = ref<AcceptanceState | null>(null);
const acceptanceLoading = ref(false);
const acceptanceError = ref("");
const acceptanceExpanded = ref(false);

type FileNode = {
  name: string;
  path: string;
  kind: "dir" | "file";
  size?: number;
  parentPath?: string;
  expanded: boolean;
  loading: boolean;
  children: FileNode[];
};

const filesOpen = ref(false);
const filesBase = ref("");
const filesRoot = ref<FileNode | null>(null);
const filesLoading = ref(false);
const filesError = ref("");
const filesNotice = ref("");

const filesSelectedPath = ref("");
const filesSelectedKind = ref<"" | "dir" | "file">("");
const filesView = ref<"preview" | "edit">("preview");

const filesFileSize = ref<number>(0);
const filesFileTruncated = ref(false);
const filesFileContent = ref("");
const filesFileOriginal = ref("");
const filesFileLoading = ref(false);
const filesFileError = ref("");
const filesSaving = ref(false);

const filesSidebarWidth = ref(340);
const filesResizing = ref(false);

function startFilesResize(e: MouseEvent) {
  filesResizing.value = true;
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";

  const startX = e.clientX;
  const startWidth = filesSidebarWidth.value;

  const onMove = (tm: MouseEvent) => {
    const diff = tm.clientX - startX;
    let newW = startWidth + diff;
    if (newW < 200) newW = 200;
    if (newW > 800) newW = 800;
    filesSidebarWidth.value = newW;
  };

  const onUp = () => {
    filesResizing.value = false;
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    window.removeEventListener("mousemove", onMove);
    window.removeEventListener("mouseup", onUp);
  };

  window.addEventListener("mousemove", onMove);
  window.addEventListener("mouseup", onUp);
}

const filesDirty = computed(
  () => filesFileContent.value !== filesFileOriginal.value,
);

const secretaryOpen = ref(false);
const secretaryView = ref<"chat" | "overview">("chat");
const secretaryScope = ref<"current" | "all">("current");
const secretaryFull = ref(false);
const secretaryWidth = ref(1100);
const secretaryResizing = ref(false);

function startSecretaryResize(e: MouseEvent) {
  if (secretaryFull.value) return;
  secretaryResizing.value = true;
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";

  const startX = e.clientX;
  const startWidth = secretaryWidth.value;

  const onMove = (tm: MouseEvent) => {
    const diff = startX - tm.clientX;
    let newW = startWidth + diff;
    const maxW = Math.min(1600, window.innerWidth - 32);
    if (newW < 520) newW = 520;
    if (newW > maxW) newW = maxW;
    secretaryWidth.value = newW;
  };

  const onUp = () => {
    secretaryResizing.value = false;
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    window.removeEventListener("mousemove", onMove);
    window.removeEventListener("mouseup", onUp);
    saveInt(LS_KEY_SECRETARY_WIDTH, secretaryWidth.value);
  };

  window.addEventListener("mousemove", onMove);
  window.addEventListener("mouseup", onUp);
}

const {
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
} = useLiveFeed({
  logsByTask,
  eventsLastEventMs,
  getCurrentRunIDs: () => selectedSession.value?.runs.map((r) => r.id) ?? [],
  loadLogs,
  onResizeEnd: (width) => {
    saveInt(LS_KEY_LIVE_WIDTH, width);
  },
});

const feedCoachDismissed = ref(false);
const feedCoachOpen = ref(false);

const runsOpen = ref(false);

const isPhone = ref(false);
const sessionsDrawerOpen = ref(false);
const sessionsFiltersOpen = ref(false);

const sessionActionsMenuOpen = ref(false);
const sessionActionsMenuSession = ref<SessionGroup | null>(null);
const sessionActionsMenuAnchor = ref<HTMLElement | null>(null);
const sessionActionsMenuPos = ref({ left: 0, top: 0 });
const sessionActionsMenuEl = ref<HTMLDivElement | null>(null);

const LS_KEY_AUTO_DELIVERY_FOREMAN = "controlccx.auto_delivery_foreman.v1";
const LS_KEY_DELIVERY_FOREMAN_SEEN = "controlccx.delivery_foreman.seen_runs.v1";
const LS_KEY_CLAUDE_AUTO_APPROVE_LEGACY = "controlccx.claude.auto_approve.v1";
const LS_KEY_MIGRATE_CLAUDE_DEFAULT_SAFETY_PRESET = "controlccx.migrate.claude_default_safety_preset.v1";
const LS_KEY_RUN_SAFETY_INTENT_BY_TOOL = "controlccx.run_safety.intent_by_tool.v1";
const LS_KEY_RUN_SAFETY_PRESET_BY_TOOL = "controlccx.run_safety.preset_by_tool.v1";
const LS_KEY_RUN_SAFETY_AUTOPILOT = "controlccx.run_safety.autopilot.v1";
const LS_KEY_RUN_SAFETY_INSTALL_UNLOCK = "controlccx.run_safety.install_unlock.v1";
const LS_KEY_ATTENTION_AUTOPILOT = "controlccx.attention_autopilot.v1";
const LS_KEY_ATTENTION_AUTOPILOT_SEEN = "controlccx.attention_autopilot.seen.v1";
const LS_KEY_WORKSPACE_MERGE_PROMPT_SEEN = "controlccx.workspace.merge_prompt_seen.v1";
const LS_KEY_REHYDRATE_PROMPT_SEEN = "controlccx.rehydrate_prompt_seen.v1";

const autoDeliveryForeman = ref<boolean>(true);
const deliveryForemanSeenRuns = ref<Set<string>>(new Set());
const deliveryForemanRunning = ref(false);
const deliveryForemanQueue = ref<Task[]>([]);
const deliveryForemanToast = ref("");
const deliveryForemanToastOpen = ref(false);

const attentionAutopilotEnabled = ref<boolean>(true);
const attentionAutopilotRunning = ref(false);
const attentionAutopilotQueue = ref<string[]>([]);
const attentionAutopilotQueued = new Set<string>();
const attentionAutopilotSeen = ref<Record<string, string>>({});
const attentionAutopilotNote = ref("");

const resumeOriginByRunID = new Map<string, ResumeOrigin>();

const workspaceMergePromptSeenRuns = ref<Set<string>>(new Set());
const workspaceMergePromptOpen = ref(false);
const workspaceMergePromptBusy = ref(false);
const workspaceMergePromptError = ref("");
const workspaceMergePromptRunID = ref("");
const workspaceMergePromptWorkspace = ref<SessionWorkspace | null>(null);

const rehydratePromptSeenRuns = ref<Set<string>>(new Set());
const rehydratePromptOpen = ref(false);
const rehydratePromptBusy = ref(false);
const rehydratePromptError = ref("");
const rehydratePromptRunID = ref("");
const rehydratePromptWorkspace = ref<SessionWorkspace | null>(null);

const runSafetyPresetByTool = ref<Record<string, string>>({});
const runSafetyIntentByTool = ref<Record<string, string>>({});
const runSafetyAutopilotEnabled = ref<boolean>(true);
const runSafetyInstallUnlock = ref<boolean>(false);

function formatLogTime(ts: string): string {
  const s = (ts ?? "").trim();
  if (!s) return "";
  const ms = Date.parse(s);
  if (!Number.isFinite(ms)) {
    // Fallback to previous behavior if the timestamp isn't parseable.
    return s.length >= 19 ? s.slice(11, 19) : s;
  }
  const d = new Date(ms);
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(d);
}

function pad2(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

function formatLocalDateTime(ts: string): string {
  const s = (ts ?? "").trim();
  if (!s) return "";
  const ms = Date.parse(s);
  if (!Number.isFinite(ms)) return s;
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`;
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

function escapeHtml(s: string): string {
  return (s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function normalizeFilePathRef(raw: string): string {
  const t = (raw ?? "").trim();
  if (!t) return "";

  // Support common "path#L12" formats.
  const hashIdx = t.indexOf("#");
  if (hashIdx > 0) {
    const base = t.slice(0, hashIdx).trim();
    if (base) return base;
  }

  // Support "path:line", "path:line-col", "path:line:col" formats.
  // Avoid clobbering Windows drive "C:\..." by requiring a colon after the drive.
  if (/^[a-z]:[\\/]/i.test(t)) {
    const lastColon = t.lastIndexOf(":");
    if (lastColon <= 1) return t;
    const suffix = t.slice(lastColon + 1);
    if (/^\d+(-\d+)?$/.test(suffix)) return t.slice(0, lastColon).trim();
    if (/^\d+:\d+$/.test(suffix)) return t.slice(0, lastColon).trim();
    return t;
  }

  const lastColon = t.lastIndexOf(":");
  if (lastColon > 0) {
    const suffix = t.slice(lastColon + 1);
    if (/^\d+(-\d+)?$/.test(suffix)) return t.slice(0, lastColon).trim();
    if (/^\d+:\d+$/.test(suffix)) return t.slice(0, lastColon).trim();
  }

  return t;
}

function looksLikeFilePath(s: string): boolean {
  const t = (s ?? "").trim();
  if (!t) return false;
  if (t.length > 260) return false;
  if (t.includes("\n") || t.includes("\r")) return false;

  const lower = t.toLowerCase();
  if (
    lower.startsWith("http://") ||
    lower.startsWith("https://") ||
    lower.startsWith("mailto:") ||
    lower.startsWith("data:")
  ) {
    return false;
  }

  if (
    lower.startsWith("results/") ||
    lower.startsWith("results\\") ||
    lower.startsWith("./") ||
    lower.startsWith("../") ||
    lower.startsWith("~/") ||
    lower.startsWith("/") ||
    /^[a-z]:[\\/]/i.test(t)
  ) {
    return true;
  }

  if (t.includes("/") || t.includes("\\")) return true;
  if (/\.[a-z0-9]{1,8}$/i.test(t)) return true;
  return false;
}

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
  breaks: true,
});

const defaultFence = md.renderer.rules.fence;
md.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  const info = (token.info ?? "").trim().toLowerCase();
  if (info === "mermaid") {
    // Mermaid will consume plain text. Keep it escaped to avoid HTML injection.
    return `<div class="mermaid">${escapeHtml(token.content)}</div>`;
  }
  // No syntax highlighting: keep dependencies slim and avoid runtime issues.
  const lang = info.split(/\s+/)[0];
  const cls = lang ? `language-${escapeHtml(lang)}` : "";
  return `<pre><code class="${cls}">${escapeHtml(token.content)}</code></pre>`;
  if (defaultFence) return defaultFence(tokens, idx, options, env, self);
  return self.renderToken(tokens, idx, options);
};

const defaultInlineCode = md.renderer.rules.code_inline;
md.renderer.rules.code_inline = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  const text = (token.content ?? "").trim();
  if (looksLikeFilePath(text)) {
    const escaped = escapeHtml(text);
    return `<code class="fileRef" data-file-path="${escaped}">${escaped}</code>`;
  }
  if (defaultInlineCode) return defaultInlineCode(tokens, idx, options, env, self);
  return self.renderToken(tokens, idx, options);
};

const selectedResultHtml = computed(() => {
  const text = selectedResultText.value ?? "";
  if (!text.trim()) return "";
  try {
    return md.render(text);
  } catch {
    return `<pre>${escapeHtml(text)}</pre>`;
  }
});

function renderMarkdownSafe(text: string): string {
  const raw = String(text ?? "");
  if (!raw.trim()) return "";
  try {
    return md.render(raw);
  } catch {
    return `<pre>${escapeHtml(raw)}</pre>`;
  }
}

function wrapHtmlForPreview(html: string): string {
  const raw = (html ?? "").trim();
  if (!raw) return "";
  const lower = raw.toLowerCase();
  if (lower.includes("<html") || lower.includes("<!doctype")) return raw;
  const bg = theme.value === "dark" ? "#0f172a" : "#ffffff";
  const fg = theme.value === "dark" ? "#e5e7eb" : "#0f172a";
  const a = theme.value === "dark" ? "#2dd4bf" : "#0d9488";
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style>
      :root { color-scheme: ${theme.value === "dark" ? "dark" : "light"}; }
      body { margin: 16px; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; background: ${bg}; color: ${fg}; }
      a { color: ${a}; }
      pre, code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; }
      pre { white-space: pre-wrap; }
      img { max-width: 100%; height: auto; }
      table { border-collapse: collapse; }
      th, td { border: 1px solid rgba(148, 163, 184, 0.35); padding: 6px 10px; }
    </style>
  </head>
  <body>${raw}</body>
</html>`;
}

const selectedResultRawHtml = computed(() => {
  const text = selectedResultText.value ?? "";
  return escapeHtml(text);
});

const selectedResultHtmlSrcDoc = computed(() => {
  const text = selectedResultText.value ?? "";
  return wrapHtmlForPreview(text);
});

const filePreviewIsMarkdown = computed(() => {
  const p = (filePreviewResolvedPath.value || filePreviewRawPath.value).trim().toLowerCase();
  return p.endsWith(".md") || p.endsWith(".markdown");
});

const filePreviewMarkdownHtml = computed(() => {
  const text = filePreviewContent.value ?? "";
  if (!text.trim()) return "";
  try {
    return md.render(text);
  } catch {
    return `<pre>${escapeHtml(text)}</pre>`;
  }
});

function highlightLangFromPath(path: string): string | null {
  const p = (path ?? "").toLowerCase();
  if (p.endsWith(".ts") || p.endsWith(".mts") || p.endsWith(".cts")) return "typescript";
  if (p.endsWith(".tsx")) return "tsx";
  if (p.endsWith(".js") || p.endsWith(".mjs") || p.endsWith(".cjs")) return "javascript";
  if (p.endsWith(".jsx")) return "jsx";
  if (p.endsWith(".vue")) return "xml";
  if (p.endsWith(".json")) return "json";
  if (p.endsWith(".yaml") || p.endsWith(".yml")) return "yaml";
  if (p.endsWith(".toml")) return "toml";
  if (p.endsWith(".md") || p.endsWith(".markdown")) return "markdown";
  if (p.endsWith(".go")) return "go";
  if (p.endsWith(".py")) return "python";
  if (p.endsWith(".sh") || p.endsWith(".bash") || p.endsWith(".zsh")) return "bash";
  if (p.endsWith(".ps1")) return "powershell";
  if (p.endsWith(".sql")) return "sql";
  if (p.endsWith(".css")) return "css";
  if (p.endsWith(".html") || p.endsWith(".htm")) return "xml";
  if (p.endsWith(".xml")) return "xml";
  if (p.endsWith(".diff") || p.endsWith(".patch")) return "diff";
  if (p.endsWith(".txt") || p.endsWith(".log")) return "plaintext";
  return null;
}

const filePreviewCodeHtml = computed(() => {
  const text = filePreviewContent.value ?? "";
  if (!text) return "";
  const path = filePreviewResolvedPath.value || filePreviewRawPath.value;
  const lang = highlightLangFromPath(path);
  try {
    if (lang === "plaintext") return escapeHtml(text);
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(text, { language: lang, ignoreIllegals: true }).value;
    }
    return hljs.highlightAuto(text).value;
  } catch {
    return escapeHtml(text);
  }
});

const filePreviewRawHtml = computed(() => {
  const text = filePreviewContent.value ?? "";
  return escapeHtml(text);
});

const filePreviewHtmlSrcDoc = computed(() => {
  const text = filePreviewContent.value ?? "";
  return wrapHtmlForPreview(text);
});

const filesIsMarkdown = computed(() => {
  if (filesSelectedKind.value !== "file") return false;
  const p = (filesSelectedPath.value ?? "").trim().toLowerCase();
  return p.endsWith(".md") || p.endsWith(".markdown");
});

const filesPreviewHtml = computed(() => {
  const text = filesFileContent.value ?? "";
  if (!text.trim()) return "";
  try {
    return md.render(text);
  } catch {
    return `<pre>${escapeHtml(text)}</pre>`;
  }
});

const filesCodeHtml = computed(() => {
  const text = filesFileContent.value ?? "";
  if (!text) return "";
  const path = filesSelectedPath.value;
  const lang = highlightLangFromPath(path);
  try {
    if (lang === "plaintext") return escapeHtml(text);
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(text, { language: lang, ignoreIllegals: true }).value;
    }
    return hljs.highlightAuto(text).value;
  } catch {
    return escapeHtml(text);
  }
});

const filteredLogs = computed(() => {
  const q = logSearch.value.trim().toLowerCase();
  const list = selectedLogs.value.filter((l) => {
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
  // Newest first for easier scanning.
  return list.slice().reverse();
});

function highlightJsonHtml(text: string): string {
  const raw = String(text ?? "");
  if (!raw) return "";
  try {
    return hljs.highlight(raw, { language: "json", ignoreIllegals: true }).value;
  } catch {
    return escapeHtml(raw);
  }
}

const prettyLogs = computed(() => {
  return filteredLogs.value.map((l) => {
    const pretty = prettifyLogMessage(l.message ?? "");
    const jsonHtml = pretty.kind === "json" && pretty.prettyJson ? highlightJsonHtml(pretty.prettyJson) : "";
    return {
      id: l.id,
      time: l.time,
      stream: l.stream,
      summary: pretty.summary,
      details: pretty.details,
      kind: pretty.kind,
      jsonHtml,
    };
  });
});

const filesVisibleNodes = computed(() => {
  const root = filesRoot.value;
  if (!root) return [] as Array<{ node: FileNode; depth: number }>;
  const out: Array<{ node: FileNode; depth: number }> = [];
  const walk = (nodes: FileNode[], depth: number) => {
    for (const n of nodes) {
      out.push({ node: n, depth });
      if (n.kind === "dir" && n.expanded && n.children.length) {
        walk(n.children, depth + 1);
      }
    }
  };
  walk(root.children, 0);
  return out;
});

async function copySelectedResult() {
  const text = selectedResultText.value;
  if (!text) return;
  await copyText(text);
}

function selectedLogStreams(): string[] {
  const out: string[] = [];
  if (logShowStdout.value) out.push("stdout");
  if (logShowStderr.value) out.push("stderr");
  if (logShowSystem.value) out.push("system");
  if (logShowAssistant.value) out.push("assistant");
  return out;
}

function downloadSelectedLogs() {
  const t = selectedTask.value;
  if (!t) return;
  const qs = new URLSearchParams();
  const streams = selectedLogStreams();
  if (streams.length && streams.length < 4) qs.set("streams", streams.join(","));
  if (logSearch.value.trim()) qs.set("q", logSearch.value.trim());
  const url = `/api/tasks/${encodeURIComponent(t.id)}/logs/export?${qs.toString()}`;
  window.open(url, "_blank", "noopener,noreferrer");
}

async function copyFilteredLogs() {
  if (!filteredLogs.value.length) return;
  const text = filteredLogs.value
    .map((l) => `${formatLocalDateTime(l.time)}\t${l.stream}\t${l.message}`)
    .join("\n");
  await copyText(text);
}

async function replaySelectedRun() {
  const t = selectedTask.value;
  if (!t) return;
  if (!confirm("Replay this run (create a new task with the same tool/workdir/prompt)?")) return;
  errorBanner.value = "";
  try {
    const next = await createTask({
      worker_type: t.worker_type,
      prompt: t.prompt,
      workdir: t.workdir,
      unsafe_automation: t.unsafe_automation || undefined,
    });
    upsertTask(next);
    selectedTaskId.value = next.id;
    await loadLogs(next.id);
  } catch (e: any) {
    const msg = e?.message ?? String(e);
    if (attentionAutopilotIsNoConversationFound(msg)) {
      stopAttentionAutopilotForSession(sessionKeyForTask(s.latest));
      errorBanner.value =
        "Resume 失败：Claude 找不到该 session（No conversation found）。建议：直接 New Run 重新开始；或检查 Claude Code 会话是否被清理/禁用持久化。原始错误：" +
        msg;
      return;
    }
    errorBanner.value = msg;
  }
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

function closeFilePreview() {
  filePreviewOpen.value = false;
  filePreviewLoading.value = false;
  filePreviewError.value = "";
  filePreviewContent.value = "";
  filePreviewRawPath.value = "";
  filePreviewBase.value = "";
  filePreviewResolvedPath.value = "";
  filePreviewSize.value = 0;
  filePreviewTruncated.value = false;
  filePreviewTab.value = "preview";
}

function dirnameForBase(path: string): string {
  const s = (path ?? "").replaceAll("\\", "/");
  const idx = s.lastIndexOf("/");
  if (idx < 0) return ".";
  if (idx === 0) return "/";
  return s.slice(0, idx);
}

async function openFilePreview(path: string, base: string) {
  const p = (path ?? "").trim();
  if (!p) return;
  filePreviewOpen.value = true;
  filePreviewLoading.value = true;
  filePreviewError.value = "";
  filePreviewRawPath.value = p;
  filePreviewBase.value = base ?? "";
  filePreviewContent.value = "";
  filePreviewResolvedPath.value = "";
  filePreviewSize.value = 0;
  filePreviewTruncated.value = false;
  try {
    const res = await fetchFSRead(p, base);
    filePreviewResolvedPath.value = res.path;
    filePreviewSize.value = res.size ?? 0;
    filePreviewTruncated.value = Boolean(res.truncated);
    filePreviewContent.value = res.content ?? "";
  } catch (e: any) {
    filePreviewError.value = e?.message ?? String(e);
  } finally {
    filePreviewLoading.value = false;
  }
}

function filePathFromClickTarget(target: EventTarget | null): string | null {
  const el = target as HTMLElement | null;
  if (!el) return null;

  const code = el.closest<HTMLElement>("[data-file-path]");
  if (code) {
    const p = (code.getAttribute("data-file-path") ?? "").trim();
    return p || null;
  }

  const a = el.closest<HTMLAnchorElement>("a[href]");
  if (a) {
    const href = (a.getAttribute("href") ?? "").trim();
    if (!href || href.startsWith("#")) return null;
    if (looksLikeFilePath(href)) return href;
  }

  return null;
}

async function onResultMarkdownClick(e: MouseEvent) {
  const path = filePathFromClickTarget(e.target);
  if (!path) return;
  e.preventDefault();
  e.stopPropagation();
  const base = selectedTask.value?.workdir ?? ".";
  await openFilePreview(normalizeFilePathRef(path), base);
}

async function onFilePreviewMarkdownClick(e: MouseEvent) {
  const path = filePathFromClickTarget(e.target);
  if (!path) return;
  e.preventDefault();
  e.stopPropagation();
  const current = filePreviewResolvedPath.value || filePreviewRawPath.value;
  const base = dirnameForBase(current);
  await openFilePreview(normalizeFilePathRef(path), base);
}

async function onFilesPreviewMarkdownClick(e: MouseEvent) {
  const path = filePathFromClickTarget(e.target);
  if (!path) return;
  e.preventDefault();
  e.stopPropagation();
  const current = filesSelectedPath.value || ".";
  const base = dirnameForBase(current);
  await openFilesFile(normalizeFilePathRef(path), base);
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
const LS_KEY_PINNED_WORKSPACE_NAMES = "controlccx.pinned_workspace_names.v1";
const LS_KEY_WORKSPACE_FILTER = "controlccx.workspace_filter.v1";
const LS_KEY_WORKSPACE_FILTERS = "controlccx.workspace_filters.v1";
const LS_KEY_CHAT_BACKEND = "controlccx.chat.backend.v1";
const LS_KEY_CHAT_STREAM = "controlccx.chat.stream.v1";
const LS_KEY_CHAT_MAX_STEPS = "controlccx.chat.max_steps.v1";
const LS_KEY_SECRETARY_VIEW = "controlccx.secretary.view.v1";
const LS_KEY_SECRETARY_SCOPE = "controlccx.secretary.scope.v1";
const LS_KEY_SECRETARY_FULL = "controlccx.secretary.full.v1";
const LS_KEY_SECRETARY_WIDTH = "controlccx.secretary.width.v1";
const LS_KEY_THEME = "controlccx.theme.v1";
const LS_KEY_FEED_SCOPE = "controlccx.feed.scope.v1";
const LS_KEY_FEED_WRAP = "controlccx.feed.wrap.v1";
const LS_KEY_FEED_MODE = "controlccx.feed.mode.v1";
const LS_KEY_LIVE_FULL = "controlccx.live.full.v1";
const LS_KEY_LIVE_WIDTH = "controlccx.live.width.v1";
const LS_KEY_COACH_FEED = "controlccx.coach.feed.v1";
const LS_KEY_SHOW_DELETED_SESSIONS = "controlccx.sessions.show_deleted.v1";

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

function loadStringMap(key: string): Record<string, string> {
  const st = getLocalStorage();
  if (!st) return {};
  try {
    const raw = st.getItem(key);
    if (!raw) return {};
    const v = JSON.parse(raw);
    if (!v || typeof v !== "object" || Array.isArray(v)) return {};
    const out: Record<string, string> = {};
    for (const [k, val] of Object.entries(v as Record<string, any>)) {
      if (typeof k !== "string") continue;
      if (typeof val !== "string") continue;
      const kk = k.trim();
      const vv = val.trim();
      if (!kk || !vv) continue;
      out[kk] = vv;
    }
    return out;
  } catch {
    return {};
  }
}

function saveStringMap(key: string, value: Record<string, string>) {
  const st = getLocalStorage();
  if (!st) return;
  try {
    st.setItem(key, JSON.stringify(value ?? {}));
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
const pinnedWorkspaceNames = ref<Record<string, string>>(
  loadStringMap(LS_KEY_PINNED_WORKSPACE_NAMES),
);
const workspaceFilters = ref<string[]>(loadStringArray(LS_KEY_WORKSPACE_FILTERS));
const workspaceSelect = ref<string>(loadString(LS_KEY_WORKSPACE_FILTER));

{
  const v = loadString(LS_KEY_CHAT_BACKEND).trim();
  if (v === "auto" || v === "claude" || v === "codex")
    chatBackend.value = v as any;
  chatStreamEnabled.value = loadBool(LS_KEY_CHAT_STREAM, true);
  const n = loadInt(LS_KEY_CHAT_MAX_STEPS, 8);
  chatMaxSteps.value = Math.max(1, Math.min(32, n));

  const sec = loadString(LS_KEY_SECRETARY_VIEW).trim();
  if (sec === "chat" || sec === "overview") secretaryView.value = sec;

	  const sc = loadString(LS_KEY_SECRETARY_SCOPE).trim();
	  if (sc === "current" || sc === "all") secretaryScope.value = sc as any;

	  secretaryFull.value = loadBool(LS_KEY_SECRETARY_FULL, false);
	  {
	    const maxW = typeof window !== "undefined" ? Math.max(520, window.innerWidth - 32) : 1600;
	    const sw = loadInt(LS_KEY_SECRETARY_WIDTH, 1100);
	    secretaryWidth.value = Math.max(520, Math.min(maxW, Math.min(1600, sw)));
	  }

  autoDeliveryForeman.value = loadBool(LS_KEY_AUTO_DELIVERY_FOREMAN, true);
  deliveryForemanSeenRuns.value = new Set(loadStringArray(LS_KEY_DELIVERY_FOREMAN_SEEN));
  workspaceMergePromptSeenRuns.value = new Set(loadStringArray(LS_KEY_WORKSPACE_MERGE_PROMPT_SEEN));
  rehydratePromptSeenRuns.value = new Set(loadStringArray(LS_KEY_REHYDRATE_PROMPT_SEEN));

  runSafetyIntentByTool.value = loadStringMap(LS_KEY_RUN_SAFETY_INTENT_BY_TOOL);
  runSafetyPresetByTool.value = loadStringMap(LS_KEY_RUN_SAFETY_PRESET_BY_TOOL);
  runSafetyAutopilotEnabled.value = loadBool(LS_KEY_RUN_SAFETY_AUTOPILOT, true);
  runSafetyInstallUnlock.value = loadBool(LS_KEY_RUN_SAFETY_INSTALL_UNLOCK, false);

  // Back-compat: v1 Claude auto-approve was a single boolean toggle.
  const legacyClaudeAutoApprove = loadBool(LS_KEY_CLAUDE_AUTO_APPROVE_LEGACY, false);
  if (legacyClaudeAutoApprove && !runSafetyPresetByTool.value["claude-code"]) {
    runSafetyPresetByTool.value = {
      ...runSafetyPresetByTool.value,
      "claude-code": "unsafe",
    };
  }
  // Migration: older versions defaulted Claude Code runs to "no-network". The intended
  // default is "search-browse" (WebFetch enabled) while still denying curl/wget.
  if (!loadBool(LS_KEY_MIGRATE_CLAUDE_DEFAULT_SAFETY_PRESET, false)) {
    if (runSafetyPresetByTool.value["claude-code"] === "no-network") {
      runSafetyPresetByTool.value = {
        ...runSafetyPresetByTool.value,
        "claude-code": "search-browse",
      };
    }
    saveBool(LS_KEY_MIGRATE_CLAUDE_DEFAULT_SAFETY_PRESET, true);
  }

  attentionAutopilotEnabled.value = loadBool(LS_KEY_ATTENTION_AUTOPILOT, true);
  attentionAutopilotSeen.value = loadStringMap(LS_KEY_ATTENTION_AUTOPILOT_SEEN);

  const fs = loadString(LS_KEY_FEED_SCOPE).trim();
  if (fs === "current" || fs === "all") liveScope.value = fs;
  const fm = loadString(LS_KEY_FEED_MODE).trim();
	  if (fm === "milestones" || fm === "all") liveMode.value = fm;
	  liveWrap.value = loadBool(LS_KEY_FEED_WRAP, true);

	  liveFull.value = loadBool(LS_KEY_LIVE_FULL, false);
	  {
	    const maxW = typeof window !== "undefined" ? Math.max(520, window.innerWidth - 32) : 1600;
	    const lw = loadInt(LS_KEY_LIVE_WIDTH, 980);
	    liveWidth.value = Math.max(520, Math.min(maxW, Math.min(1600, lw)));
	  }

	  sessionsShowDeleted.value = loadBool(LS_KEY_SHOW_DELETED_SESSIONS, false);

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

  feedCoachDismissed.value = loadBool(LS_KEY_COACH_FEED, false);

  // Back-compat: if old single filter exists, seed multi filters.
  if (workspaceFilters.value.length === 0 && workspaceSelect.value.trim()) {
    workspaceFilters.value = [workspaceSelect.value.trim()];
  }
}

watch(
  pinnedWorkspaces,
  (v) => {
    saveStringArray(LS_KEY_PINNED_WORKSPACES, v);
    const pinnedKeys = new Set(v.map((p) => normalizePathForCompare(p)));
    const next: Record<string, string> = {};
    for (const [k, name] of Object.entries(pinnedWorkspaceNames.value)) {
      const key = normalizePathForCompare(k);
      if (!key) continue;
      if (!pinnedKeys.has(key)) continue;
      if (!name.trim()) continue;
      next[key] = name.trim();
    }
    // Keep storage small and avoid stale entries.
    if (JSON.stringify(next) !== JSON.stringify(pinnedWorkspaceNames.value)) {
      pinnedWorkspaceNames.value = next;
    }
  },
  { deep: true },
);
watch(
  pinnedWorkspaceNames,
  (v) => saveStringMap(LS_KEY_PINNED_WORKSPACE_NAMES, v),
  { deep: true },
);
watch(
  workspaceFilters,
  (v) => {
    saveStringArray(LS_KEY_WORKSPACE_FILTERS, v);
    // Keep legacy single value as the first one (or empty).
    saveString(LS_KEY_WORKSPACE_FILTER, v[0] ?? "");
    if ((v[0] ?? "").trim()) newWorkdir.value = v[0]!;
  },
  { deep: true },
);

watch(workspaceSelect, (v) => {
  const s = v.trim();
  if (!s) {
    workspaceFilters.value = [];
    return;
  }
  // Selecting from dropdown sets a primary workspace.
  workspaceFilters.value = [s];
});

watch(sessionsShowDeleted, (v) => {
  saveBool(LS_KEY_SHOW_DELETED_SESSIONS, v);
  void refresh();
});
watch(chatBackend, (v) => saveString(LS_KEY_CHAT_BACKEND, v));
watch(chatStreamEnabled, (v) => saveBool(LS_KEY_CHAT_STREAM, v));
watch(chatMaxSteps, (v) => saveInt(LS_KEY_CHAT_MAX_STEPS, v));
watch(secretaryView, (v) => saveString(LS_KEY_SECRETARY_VIEW, v));
watch(secretaryScope, (v) => saveString(LS_KEY_SECRETARY_SCOPE, v));
watch(secretaryFull, (v) => saveBool(LS_KEY_SECRETARY_FULL, Boolean(v)));
watch(autoDeliveryForeman, (v) => saveBool(LS_KEY_AUTO_DELIVERY_FOREMAN, Boolean(v)));
watch(
  runSafetyIntentByTool,
  (v) => saveStringMap(LS_KEY_RUN_SAFETY_INTENT_BY_TOOL, v),
  { deep: true },
);
watch(
  runSafetyPresetByTool,
  (v) => saveStringMap(LS_KEY_RUN_SAFETY_PRESET_BY_TOOL, v),
  { deep: true },
);
watch(runSafetyAutopilotEnabled, (v) => saveBool(LS_KEY_RUN_SAFETY_AUTOPILOT, Boolean(v)));
watch(runSafetyInstallUnlock, (v) => saveBool(LS_KEY_RUN_SAFETY_INSTALL_UNLOCK, Boolean(v)));
watch(attentionAutopilotEnabled, (v) => saveBool(LS_KEY_ATTENTION_AUTOPILOT, Boolean(v)));
watch(theme, (v) => saveString(LS_KEY_THEME, v));
watch(liveScope, (v) => saveString(LS_KEY_FEED_SCOPE, v));
watch(liveWrap, (v) => saveBool(LS_KEY_FEED_WRAP, v));
watch(liveMode, (v) => saveString(LS_KEY_FEED_MODE, v));
watch(liveFull, (v) => saveBool(LS_KEY_LIVE_FULL, Boolean(v)));
watch(feedCoachDismissed, (v) => saveBool(LS_KEY_COACH_FEED, v));

function desiredOutputTabForTask(t: Task | null): "result" | "logs" {
  if (!t) return "result";
  if (t.status === "running" || t.status === "queued") return "logs";
  return "result";
}

watch(selectedTaskId, () => {
  const t = selectedTask.value;
  if (!t) return;
  if (workspaceMergePromptOpen.value && workspaceMergePromptRunID.value !== selectedTaskId.value) {
    closeWorkspaceMergePrompt();
  }
  if (rehydratePromptOpen.value && rehydratePromptRunID.value !== selectedTaskId.value) {
    closeRehydratePrompt();
  }
  outputTab.value = desiredOutputTabForTask(t);
  logShowAssistant.value = true;
  logShowStdout.value = true;
  logShowStderr.value = true;
  logShowSystem.value = true;
  logSearch.value = "";
  resumeExpanded.value = false;
  resumeSafetyOverride.value = false;
});

watch(outputTab, (v) => {
  if (v !== "trace") return;
  if (!selectedTaskId.value) return;
  void loadTrace(selectedTaskId.value);
});

watch([workspaceFilters, sessionSearch], () => {
  sessionsLimit.value = 40;
});

watch(
  () => selectedTask.value?.status ?? "",
  (next, prev) => {
    if (!next) return;
    if (next === prev) return;
    const t = selectedTask.value;
    if (!t) return;
    const desired = desiredOutputTabForTask(t);
    // Auto switch only when it would help: running->logs, finished->result.
    if (outputTab.value !== desired) outputTab.value = desired;
  },
);

type SessionGroup = {
  key: string;
  session_id: string;
  title: string;
  deleted_at: string;
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

let phoneMq: MediaQueryList | null = null;
let phoneMqHandler: (() => void) | null = null;
let focusInHandler: ((e: FocusEvent) => void) | null = null;

function scrollFocusedIntoView(el: HTMLElement) {
  if (!isPhone.value) return;
  window.setTimeout(() => {
    try {
      el.scrollIntoView({ block: "center" });
    } catch {
      // ignore
    }
  }, 50);
}

async function refresh() {
  const [sys, taskList, chatList] = await Promise.all([
    fetchSystemInfo(),
    refreshTasks(200),
    fetchChat(),
  ]);
  systemInfo.value = sys;
  // If the page reloads while a session is interrupted, Autopilot should still try once.
  if (attentionAutopilotEnabled.value) {
    const keys = new Set<string>();
    for (const t of taskList) {
      if (t.status !== "interrupted") continue;
      keys.add(sessionKeyForTask(t));
    }
    for (const k of keys) enqueueAttentionAutopilot(k);
  }
  chat.value = chatList;
}

async function refreshAuth() {
  try {
    authInfo.value = await fetchAuthInfo();
  } catch {
    // ignore auth status failures (UI still works; tasks will surface logs)
  }
}

async function onCreateTask(): Promise<boolean> {
  errorBanner.value = "";
  try {
    const driver = newRunDriver.value;
    const useAutopilot = runSafetyAutopilotEnabled.value && !newRunSafetyOverride.value;

    let safety: RunSafetyPayload = {};
    if (!useAutopilot) {
      const intent = newRunTaskIntent.value;
      const preset = newRunSafetyPreset.value;

      if (
        (driver === "claude-code" || driver === "codex") &&
        isHighRiskPreset(driver, preset) &&
        !newRunHighRiskOptIn.value
      ) {
        const ok = await requestHighRiskConfirm({
          title: "高风险确认",
          message: "该运行需要高风险设置（权限更大）。继续吗？",
          detail: highRiskPresetSummary(driver, preset),
          confirmLabel: "继续（我已知晓）",
        });
        if (!ok) return false;
        newRunHighRiskOptIn.value = true;
      }

      // Persist effective choices for future runs.
      setStringMapKey(runSafetyIntentByTool, newWorkerType.value, intent);
      setStringMapKey(runSafetyPresetByTool, newWorkerType.value, preset);

      safety = buildRunSafetyPayload(driver, intent, preset);
    }

    const envelope = buildSafetyEnvelopePayload();

    const t = await createTask({
      worker_type: newWorkerType.value,
      prompt: newPrompt.value,
      workdir: newWorkdir.value,
      ...envelope,
      ...safety,
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
  closeSessionActionsMenu();
  sessionsDrawerOpen.value = false;
  await selectTask(id, {
    closeMobileDrawer: isPhone.value ? () => { sessionsDrawerOpen.value = false; } : undefined,
    closeRunsModal: () => { runsOpen.value = false; },
  });
}

const sessionRenameOpen = ref(false);
const sessionRenameKey = ref("");
const sessionRenameTitle = ref("");
const sessionRenameError = ref("");
const sessionRenameSaving = ref(false);
const sessionRenameInputEl = ref<HTMLInputElement | null>(null);

const workspaceRenameOpen = ref(false);
const workspaceRenamePath = ref("");
const workspaceRenameValue = ref("");
const workspaceRenameInputEl = ref<HTMLInputElement | null>(null);

const sessionDeleteOpen = ref(false);
const sessionDeleteKey = ref("");
const sessionDeleteLabel = ref("");
const sessionDeleteError = ref("");
const sessionDeleteSaving = ref(false);

function sessionShortID(s: SessionGroup): string {
  return (s.session_id || s.latest.id).slice(0, 8);
}

function sessionDisplayLabel(s: SessionGroup): string {
  const t = (s.title ?? "").trim();
  if (t) return t;
  return sessionShortID(s);
}

function toSingleLine(text: string): string {
  return (text ?? "").replace(/\s+/g, " ").trim();
}

function shortPathTail(path: string, segments = 2): string {
  const raw = (path ?? "").trim();
  if (!raw) return "";
  const norm = normalizePathForCompare(raw);
  if (!norm) return raw;
  if (norm === "/") return "/";
  const parts = norm.split("/").filter(Boolean);
  if (parts.length === 0) return raw;
  const tail = parts.slice(-Math.max(1, segments)).join("/");
  return parts.length > segments ? `…/${tail}` : tail;
}

function workdirLabelForSession(workdir: string): string {
  const raw = (workdir ?? "").trim();
  if (!raw) return "";
  const dir = normalizePathForCompare(raw);
  if (!dir) return raw;

  // Prefer the longest pinned workspace prefix with a user-defined name.
  let bestRoot = "";
  let bestName = "";
  for (const p of pinnedWorkspaces.value) {
    const root = normalizePathForCompare(p);
    if (!root) continue;
    if (!(dir === root || dir.startsWith(root + "/"))) continue;
    const name = pinnedWorkspaceName(p);
    if (!name) continue;
    if (root.length > bestRoot.length) {
      bestRoot = root;
      bestName = name;
    }
  }

  if (!bestName) return shortPathTail(dir, 2);
  if (dir === bestRoot) return bestName;

  let rel = dir.slice(bestRoot.length);
  if (rel.startsWith("/")) rel = rel.slice(1);
  const relParts = rel.split("/").filter(Boolean);
  if (relParts.length === 0) return bestName;

  const relTail =
    relParts.length <= 2
      ? relParts.join("/")
      : `…/${relParts.slice(-2).join("/")}`;
  return `${bestName}/${relTail}`;
}

function promptSummary(text: string): string {
  return toSingleLine(text);
}

function openSessionRename(s: SessionGroup) {
  sessionRenameError.value = "";
  sessionRenameKey.value = s.key;
  sessionRenameTitle.value = (s.title ?? "").trim();
  sessionRenameOpen.value = true;
  void nextTick(() => sessionRenameInputEl.value?.focus());
}

function closeSessionRename() {
  sessionRenameOpen.value = false;
}

async function saveSessionRename() {
  const key = sessionRenameKey.value.trim();
  if (!key) return;
  sessionRenameSaving.value = true;
  sessionRenameError.value = "";
  try {
    await renameSession(key, sessionRenameTitle.value);
    sessionRenameOpen.value = false;
    await refresh();
  } catch (e: any) {
    sessionRenameError.value = e?.message ?? String(e);
  } finally {
    sessionRenameSaving.value = false;
  }
}

function openSessionDelete(s: SessionGroup) {
  sessionDeleteError.value = "";
  sessionDeleteKey.value = s.key;
  sessionDeleteLabel.value = sessionDisplayLabel(s);
  sessionDeleteOpen.value = true;
}

function closeSessionDelete() {
  sessionDeleteOpen.value = false;
}

async function confirmSessionDelete() {
  const key = sessionDeleteKey.value.trim();
  if (!key) return;
  sessionDeleteSaving.value = true;
  sessionDeleteError.value = "";
  try {
    await deleteSession(key);
    sessionDeleteOpen.value = false;
    await refresh();
  } catch (e: any) {
    sessionDeleteError.value = e?.message ?? String(e);
  } finally {
    sessionDeleteSaving.value = false;
  }
}

function openNewRun() {
  newRunOpen.value = true;
  newRunSafetyOverride.value = false;
  newRunHighRiskOptIn.value = false;
  if (!toolsList.value.length) void refreshTools();
}

function closeNewRun() {
  newRunOpen.value = false;
}

function goHome() {
  closeSessionActionsMenu();
  runsOpen.value = false;
  sessionsDrawerOpen.value = false;
  selectedTaskId.value = "";
  if (skillsOpen.value) {
    closeSkillsPage();
    return;
  }
  if (filesOpen.value) {
    closeFilesPage();
    return;
  }
  navigateTo("/");
}

async function runFromHome() {
  if (homeRunBusy.value) return;
  homeRunBusy.value = true;
  try {
    const ok = await onCreateTask();
    if (ok) sessionsDrawerOpen.value = false;
  } finally {
    homeRunBusy.value = false;
  }
}

function closeSessionActionsMenu() {
  sessionActionsMenuOpen.value = false;
  sessionActionsMenuSession.value = null;
  sessionActionsMenuAnchor.value = null;
}

function positionSessionActionsMenu(opts?: { menuWidth?: number; menuHeight?: number }) {
  const anchorEl = sessionActionsMenuAnchor.value;
  if (!anchorEl) return;
  const rect = anchorEl.getBoundingClientRect();
  const menuWidth = Math.max(160, Math.min(320, opts?.menuWidth ?? 180));
  const menuHeight = Math.max(80, Math.min(420, opts?.menuHeight ?? 140));
  sessionActionsMenuPos.value = computePopupPosition({
    anchor: {
      left: rect.left,
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      width: rect.width,
      height: rect.height,
    },
    menu: { width: menuWidth, height: menuHeight },
    viewport: { width: window.innerWidth, height: window.innerHeight },
    margin: 10,
    offsetY: 8,
  });
}

function openSessionActionsMenu(s: SessionGroup, ev: MouseEvent) {
  const el = ev.currentTarget as HTMLElement | null;
  if (!el) return;
  sessionActionsMenuSession.value = s;
  sessionActionsMenuAnchor.value = el;
  sessionActionsMenuOpen.value = true;
  positionSessionActionsMenu();
  void nextTick(() => {
    const menuEl = sessionActionsMenuEl.value;
    if (!menuEl) return;
    const m = menuEl.getBoundingClientRect();
    positionSessionActionsMenu({ menuWidth: m.width, menuHeight: m.height });
  });
}

function toggleSessionActionsMenu(s: SessionGroup, ev: MouseEvent) {
  ev.stopPropagation();
  if (sessionActionsMenuOpen.value && sessionActionsMenuSession.value?.key === s.key) {
    closeSessionActionsMenu();
    return;
  }
  openSessionActionsMenu(s, ev);
}

function onSessionActionsMenuKeyDown(ev: KeyboardEvent) {
  if (!sessionActionsMenuOpen.value) return;
  if (ev.key !== "Escape") return;
  ev.preventDefault();
  closeSessionActionsMenu();
}

function onSessionActionsMenuDocumentMouseDown(ev: MouseEvent) {
  if (!sessionActionsMenuOpen.value) return;
  const t = ev.target as Node | null;
  if (!t) return;
  const menuEl = sessionActionsMenuEl.value;
  const anchorEl = sessionActionsMenuAnchor.value;
  if (menuEl && menuEl.contains(t)) return;
  if (anchorEl && anchorEl.contains(t)) return;
  closeSessionActionsMenu();
}

async function onCreateTaskFromModal() {
  if (!newPrompt.value.trim()) return;
  if (!newWorkdir.value.trim()) return;
  if (missingAuthText.value) return;
  const ok = await onCreateTask();
  if (ok) closeNewRun();
}

function toggleSecretary() {
  const next = !secretaryOpen.value;
  if (next) liveOpen.value = false;
  secretaryOpen.value = next;
}

function closeSecretary() {
  secretaryOpen.value = false;
}

function openLive() {
  feedCoachOpen.value = false;
  feedCoachDismissed.value = true;
  secretaryOpen.value = false;
  liveOpen.value = true;
}

function openRuns() {
  runsOpen.value = true;
}

function normalizeRoutePath(path: string): string {
  let p = (path ?? "").trim();
  if (!p.startsWith("/")) p = "/" + p;
  while (p.length > 1 && p.endsWith("/")) p = p.slice(0, -1);
  return p || "/";
}

function navigateTo(path: string, opts?: { replace?: boolean }) {
  if (typeof window === "undefined") return;
  const u = new URL(path, window.location.href);
  const nextPath = normalizeRoutePath(u.pathname);
  const next = nextPath + (u.search || "");
  if (window.location.pathname + window.location.search === next) return;
  if (opts?.replace) {
    window.history.replaceState({}, "", next);
  } else {
    window.history.pushState({}, "", next);
  }
}

function openInNewTab(url: string) {
  if (typeof window === "undefined") return;
  try {
    window.open(url, "_blank", "noopener,noreferrer");
  } catch {
    // ignore
  }
}

function encodePathForQueryValue(path: string): string {
  // Keep "/" readable in query string while still encoding other reserved chars safely.
  return encodeURIComponent(path).replaceAll("%2F", "/");
}

async function openSkillsPage() {
  navigateTo("/skills");
  sessionsDrawerOpen.value = false;
  await Promise.all([openSkills(), refreshSkillVersions()]);
}



function closeSkillsPage() {
  skillsOpen.value = false;
  navigateTo("/");
}

async function refreshSkillsPage() {
  await Promise.all([refreshSkills(), refreshSkillVersions()]);
}

async function openFilesForBase(base: string) {
  const b = (base ?? "").trim() || ".";
  if (filesDirty.value && !window.confirm("Discard unsaved changes?")) return;

  filesOpen.value = true;
  filesLoading.value = true;
  filesError.value = "";
  filesNotice.value = "";
  filesBase.value = b;
  filesRoot.value = null;
  resetFilesEditor();

  try {
    const res = await fetchFSEntries(".", b);
    filesBase.value = res.path;
    filesRoot.value = {
      name: res.path,
      path: res.path,
      kind: "dir",
      expanded: true,
      loading: false,
      children: (res.entries ?? []).map((e) => fsEntryToNode(e, res.path)),
    };
  } catch (e: any) {
    filesError.value = e?.message ?? String(e);
  } finally {
    filesLoading.value = false;
  }
}

async function openFilesPageFromLocation() {
  if (typeof window === "undefined") return;
  const qs = new URLSearchParams(window.location.search);
  const base = (qs.get("base") ?? "").trim();
  await openFilesForBase(base);
  if (!base.trim()) {
    filesNotice.value =
      "Tip: open Files from a session to browse its workdir, or pass ?base=/path.";
  }
}

function closeFilesPage() {
  closeFiles();
  if (filesOpen.value) return;
  navigateTo("/");
}

function applyRouteFromLocation() {
  if (typeof window === "undefined") return;
  const path = normalizeRoutePath(window.location.pathname);
  const qs = new URLSearchParams(window.location.search);
  const restoreFilesRoute = () => {
    const base = (filesBase.value ?? "").trim() || ".";
    navigateTo(`/files?base=${encodePathForQueryValue(base)}`, { replace: true });
  };

  // Back-compat / defensive: if someone lands on `/?base=...`, treat it as Files.
  if ((path === "/" || path === "/index.html") && qs.has("base")) {
    const base = (qs.get("base") ?? "").trim();
    skillsOpen.value = false;
    navigateTo(`/files?base=${encodePathForQueryValue(base)}`, { replace: true });
    void openFilesForBase(base);
    return;
  }

  if (path === "/skills") {
    if (filesOpen.value) {
      closeFiles();
      if (filesOpen.value) {
        restoreFilesRoute();
        return;
      }
    }
    void openSkillsPage();
    return;
  }
  if (path === "/files") {
    skillsOpen.value = false;
    void openFilesPageFromLocation();
    return;
  }
  if (skillsOpen.value) skillsOpen.value = false;
  if (filesOpen.value) {
    closeFiles();
    if (filesOpen.value) restoreFilesRoute();
  }
}

function onRoutePopState() {
  applyRouteFromLocation();
}

function dismissFeedCoach() {
  feedCoachDismissed.value = true;
  feedCoachOpen.value = false;
}

function closeHeaderMoreMenu() {
  const el = headerMoreEl.value;
  if (!el) return;
  try {
    el.open = false;
  } catch {
    // ignore
  }
}

function onHeaderMoreDocumentMouseDown(e: MouseEvent) {
  const el = headerMoreEl.value;
  if (!el || !el.open) return;
  const target = e.target as Node | null;
  if (!target) return;
  if (el.contains(target)) return;
  closeHeaderMoreMenu();
}

function toggleTheme() {
  applyTheme(theme.value === "dark" ? "light" : "dark");
}

function onToggleThemeFromMenu() {
  toggleTheme();
  closeHeaderMoreMenu();
}

function onOpenLiveFromMenu() {
  openLive();
  closeHeaderMoreMenu();
}

function onOpenSkillsFromMenu() {
  void openSkillsPage();
  closeHeaderMoreMenu();
}

function onOpenSettingsFromMenu() {
  openAuthSettings();
  closeHeaderMoreMenu();
}

function openSecretaryForForeman() {
  liveOpen.value = false;
  secretaryOpen.value = true;
  secretaryView.value = "chat";
  deliveryForemanToastOpen.value = false;
}

function isTerminalStatus(s: string): boolean {
  return (
    s === "succeeded" ||
    s === "failed" ||
    s === "canceled" ||
    s === "interrupted" ||
    s === "blocked"
  );
}

function persistDeliveryForemanSeen(runID: string) {
  const id = (runID ?? "").trim();
  if (!id) return;
  if (deliveryForemanSeenRuns.value.has(id)) return;
  const next = new Set(deliveryForemanSeenRuns.value);
  next.add(id);
  // Limit growth to keep localStorage small.
  const arr = Array.from(next).slice(-400);
  deliveryForemanSeenRuns.value = new Set(arr);
  saveStringArray(LS_KEY_DELIVERY_FOREMAN_SEEN, arr);
}

function showDeliveryForemanToast(message: string) {
  deliveryForemanToast.value = message;
  deliveryForemanToastOpen.value = true;
  window.setTimeout(() => {
    if (deliveryForemanToast.value === message) deliveryForemanToastOpen.value = false;
  }, 10_000);
}

function persistWorkspaceMergePromptSeen(runID: string) {
  const id = (runID ?? "").trim();
  if (!id) return;
  if (workspaceMergePromptSeenRuns.value.has(id)) return;
  const next = new Set(workspaceMergePromptSeenRuns.value);
  next.add(id);
  // Limit growth to keep localStorage small.
  const arr = Array.from(next).slice(-800);
  workspaceMergePromptSeenRuns.value = new Set(arr);
  saveStringArray(LS_KEY_WORKSPACE_MERGE_PROMPT_SEEN, arr);
}

function persistRehydratePromptSeen(runID: string) {
  const id = (runID ?? "").trim();
  if (!id) return;
  if (rehydratePromptSeenRuns.value.has(id)) return;
  const next = new Set(rehydratePromptSeenRuns.value);
  next.add(id);
  // Limit growth to keep localStorage small.
  const arr = Array.from(next).slice(-800);
  rehydratePromptSeenRuns.value = new Set(arr);
  saveStringArray(LS_KEY_REHYDRATE_PROMPT_SEEN, arr);
}

function closeWorkspaceMergePrompt() {
  workspaceMergePromptOpen.value = false;
  workspaceMergePromptBusy.value = false;
  workspaceMergePromptError.value = "";
  workspaceMergePromptRunID.value = "";
  workspaceMergePromptWorkspace.value = null;
}

function closeRehydratePrompt() {
  rehydratePromptOpen.value = false;
  rehydratePromptBusy.value = false;
  rehydratePromptError.value = "";
  rehydratePromptRunID.value = "";
  rehydratePromptWorkspace.value = null;
}

async function confirmWorkspaceMergePrompt() {
  const runID = workspaceMergePromptRunID.value.trim();
  const ws = workspaceMergePromptWorkspace.value;
  if (!runID || !ws || ws.status !== "active" || selectedTaskId.value !== runID) {
    closeWorkspaceMergePrompt();
    return;
  }
  workspaceMergePromptBusy.value = true;
  workspaceMergePromptError.value = "";
  try {
    await mergeRunWorkspace();
    if (runWorkspaceConflict.value) return; // keep modal open for user to read the conflict summary
    closeWorkspaceMergePrompt();
  } catch (e: any) {
    workspaceMergePromptError.value = e?.message ?? String(e);
  } finally {
    workspaceMergePromptBusy.value = false;
  }
}

async function confirmRehydratePrompt() {
  const runID = rehydratePromptRunID.value.trim();
  if (!runID || selectedTaskId.value !== runID) {
    closeRehydratePrompt();
    return;
  }
  if (rehydratePromptBusy.value) return;

  rehydratePromptBusy.value = true;
  rehydratePromptError.value = "";
  try {
    try {
      await refreshRunWorkspace();
    } catch {
      // ignore workspace fetch errors (server will re-check on rehydrate)
    }
    const ws = runWorkspace.value;
    rehydratePromptWorkspace.value = ws;
    if (ws && ws.status === "active") {
      rehydratePromptError.value = "请先在 Workspace 面板点击「Merge」把隔离工作区的改动合并回 base_workdir，然后再继续。";
      return;
    }

    const nt = await rehydrateTaskWithOptions(runID, { prompt: "continue" });
    upsertTask(nt);
    selectedTaskId.value = nt.id;
    await loadLogs(nt.id);
    outputTab.value = "logs";
    closeRehydratePrompt();
  } catch (e: any) {
    rehydratePromptError.value = e?.message ?? String(e);
  } finally {
    rehydratePromptBusy.value = false;
  }
}

async function maybePromptWorkspaceMerge(prev: Task | undefined, next: Task) {
  if (!prev || !next?.id) return;
  if (workspaceMergePromptOpen.value) return;
  if (workspaceMergePromptSeenRuns.value.has(next.id)) return;

  const prevStatus = prev?.status ?? "";
  const nextStatus = next.status ?? "";
  if (isTerminalStatus(prevStatus) || !isTerminalStatus(nextStatus)) return;
  if (!(prevStatus === "running" || prevStatus === "queued" || prevStatus === "")) return;

  // Non-disruptive: only prompt for the currently selected run.
  if (selectedTaskId.value !== next.id) return;

  try {
    await refreshRunWorkspace();
  } catch {
    // ignore fetch errors
  }
  const ws2 = runWorkspace.value;
  if (!ws2 || ws2.status !== "active") return;

  workspaceMergePromptRunID.value = next.id;
  workspaceMergePromptWorkspace.value = ws2;
  workspaceMergePromptError.value = "";
  workspaceMergePromptBusy.value = false;
  workspaceMergePromptOpen.value = true;
  persistWorkspaceMergePromptSeen(next.id);
}

async function maybePromptRehydrate(prev: Task | undefined, next: Task) {
  if (!prev || !next?.id) return;
  if (rehydratePromptOpen.value) return;
  if (rehydratePromptSeenRuns.value.has(next.id)) return;

  const prevStatus = prev?.status ?? "";
  const nextStatus = next.status ?? "";
  if (isTerminalStatus(prevStatus) || !isTerminalStatus(nextStatus)) return;
  if (!(prevStatus === "running" || prevStatus === "queued" || prevStatus === "")) return;

  const origin = resumeOriginByRunID.get(next.id) ?? "";
  if (!shouldOfferRehydrateForTask(next, origin)) return;

  // Non-disruptive: only prompt for the currently selected run (manual resume).
  if (selectedTaskId.value !== next.id) return;

  try {
    await refreshRunWorkspace();
  } catch {
    // ignore
  }
  rehydratePromptWorkspace.value = runWorkspace.value;
  rehydratePromptRunID.value = next.id;
  rehydratePromptError.value = "";
  rehydratePromptBusy.value = false;
  rehydratePromptOpen.value = true;
  persistRehydratePromptSeen(next.id);

  // Avoid unbounded growth.
  resumeOriginByRunID.delete(next.id);
}

async function buildDeliveryForemanPrompt(t: Task): Promise<string> {
  const runID = t.id;
  const sessionID = t.session_id?.trim() || "";
  const sessKey = sessionKeyForTask(t);
  const sess = sessionsAll.value.find((s) => s.key === sessKey);
  const runsCount = sess?.runs.length ?? 1;

  let tail = "";
  try {
    const logs = await fetchLogs(runID, 0, 200);
    const filtered = logs.filter((l) => l.stream !== "stdout");
    const last = filtered.slice(-40);
    tail = last
      .map((l) => `[${formatLogTime(l.time)} ${l.stream}] ${String(l.message ?? "").slice(0, 600)}`)
      .join("\n");
  } catch {
    // ignore logs fetch failures
  }

  const parts: string[] = [];
  parts.push("【Delivery Foreman / 交付前哨】");
  parts.push("");
  parts.push("请你作为系统秘书/观察者，判断该 run 是否真的完成，以及是否需要工业级交付检查。");
  parts.push("如果未完成：你 SHOULD 直接调用 task_resume 工具创建新的 resume run（尽量不要让用户手动操作），并在回复里说明你做了什么；同时列出需要补齐的关键点。");
  parts.push("如果你判断不适合自动 resume（例如需要用户选择/高风险/信息不足），才给出用户下一步最小 resume prompt（用户要输入的那句话）。");
  parts.push("如果已完成且属于复杂任务：给出工业级交付 checklist（以可执行步骤/命令为主，不默认执行）。");
  parts.push("如果是简单任务：一句话说明无需工业级交付检查并结束。");
  parts.push("");
  parts.push("【Acceptance Gates / 验收闸门（仅复杂任务启用）】");
  parts.push("当你判断这是复杂任务时：");
  parts.push("1) 先调用 acceptance_prepare({task_id: run_id, max_iterations: 10}) 获取 iteration i/10，并确保不会超过上限。");
  parts.push("   - 若 can_continue=false：你 MUST 升级给用户（最小下一步 + 证据），不要继续自动迭代。");
  parts.push("2) 再调用 acceptance_get({task_id: run_id}) 获取当前验收状态，避免重复触发/无限循环。");
  parts.push("3) 你 SHOULD 调用 acceptance_build_contract({task_id: run_id}) 得到 deterministic baseline 的 plan_json，再结合用户要求补齐/修正。");
  parts.push("4) 你 SHOULD 调用 acceptance_evaluate_objectives({task_id: run_id, plan_json}) 评估客观标准，并把测量值写入 report（Markdown）。");
  parts.push("5) 你 MUST 使用 acceptance_update 写入/更新验收状态（server 持久化），让 UI 可见进度与报告。");
  parts.push("");
  parts.push("【验收方法论（不要把任务硬塞进固定分类）】");
  parts.push("- 先复述交付意图：intent_summary（人类可读，不需要固定枚举）");
  parts.push("- 抽取用户显式硬约束 -> objective_criteria（可测量阈值 + 证据来源）");
  parts.push("- 把主观要求拆成 rubric -> subjective_rubrics（逐项 pass/fail + 理由 + 修改建议）");
  parts.push("- 仅在明显需要“可运行/可验证交付”时才注入默认 gates，并写明适用性与理由；不适用要跳过并记录原因");
  parts.push("");
  parts.push("【默认 runnable DoD gates（仅适用时）】");
  parts.push("- README 一键启动");
  parts.push("- go test ./...");
  parts.push("- pnpm test");
  parts.push("- 启动后首页可打开（HTTP smoke）");
  parts.push("- 可选：Playwright smoke（优先 Playwright MCP；不可用则降级 HTTP smoke，并在报告里说明）");
  parts.push("");
  parts.push("【Worker Verification Recipes（跨工具通用，写进 report 作为证据）】");
  parts.push("3.1 Project DoD recipe（仅适用 runnable deliverable）：");
  parts.push("- README：必须有 Quick Start/一键启动（给出单行命令）；没有就先补齐");
  parts.push("- Go：若存在 go.mod -> 运行 `go test ./...`（把输出保留在日志）");
  parts.push("- Node：若存在 package.json 且包含 test script -> 运行 `pnpm test`（或项目约定的 test 命令）；否则记录为 skipped（not applicable）");
  parts.push("");
  parts.push("3.2 Smoke recipe（start + HTTP smoke + stop cleanly）：");
  parts.push("- 按 README 的启动命令启动服务（尽量前台运行，便于 Ctrl+C 退出）");
  parts.push("- 发现端口后，用 `curl -fsS http://127.0.0.1:<port>/`（或 /health）确认可访问（记录 HTTP 状态码/响应片段）");
  parts.push("- 停止服务（Ctrl+C），确认没有残留进程/端口占用（记录结果）");
  parts.push("");
  parts.push("3.3 Optional: Playwright MCP smoke（优先 MCP；不可用则降级）：");
  parts.push("- 若环境提供 Playwright MCP：打开首页并断言关键 UI 可见（截图/断言结果作为证据）");
  parts.push("- 若不可用：降级为 3.2 的 HTTP smoke，并在 report 里写明“为何降级”");
  parts.push("");
  parts.push("【plan_json schema（作为 JSON 字符串写入 acceptance_update.plan_json）】");
  parts.push(
    `{
  "intent_summary": "...",
  "complexity_reason": "...",
  "objective_criteria": [
    {"id":"word_count","title":">=30000 words","method":"task_output_stats.words","min":30000},
    {"id":"sections","title":"14 parts","method":"task_output_stats.sections","min":14}
  ],
  "subjective_rubrics": [
    {"id":"wechat","title":"适合公众号","items":[{"item":"标题/结构/可读性/CTA/合规提示","pass_criteria":"..."}]}
  ],
  "default_gates": [
    {"id":"runnable.dod","applies_if":"deliverable must run","reason":"..."}
  ]
}`,
  );
  parts.push("");
  parts.push("【写入时序（示例）】");
  parts.push("- 第一次：acceptance_update({task_id, status:'running', iteration:i, max_iterations:10, current_gate:'contract', summary:'...', plan_json:'{...}'})");
  parts.push("- 评估中：acceptance_update({task_id, status:'running', current_gate:'...', summary:'...'})");
  parts.push("- 验收通过：acceptance_update({task_id, status:'accepted', current_gate:'done', summary:'...', report:'...markdown...'})");
  parts.push("- 验收失败/需用户介入/到达上限：acceptance_update({task_id, status:'failed', summary:'...', report:'...markdown...'})");
  parts.push("注意：不要重复刷屏；每轮只汇报新增信息，并确保用户能看到 iteration i/10 的进展。");
  parts.push("");
  parts.push("【上下文】");
  parts.push(`run_id: ${runID}`);
  if (sessionID) parts.push(`session_id: ${sessionID}`);
  parts.push(`worker: ${t.worker_type}`);
  parts.push(`status: ${t.status}`);
  parts.push(`workdir: ${t.workdir}`);
  parts.push(`runs_in_session: ${runsCount}`);
  if (t.warning) parts.push(`warning: ${t.warning}`);
  if (t.error) parts.push(`error: ${t.error}`);
  parts.push("");
  parts.push("prompt:");
  parts.push(t.prompt || "");
  if (tail) {
    parts.push("");
    parts.push("recent_logs_tail:");
    parts.push(tail);
  }
  return parts.join("\n");
}

async function runDeliveryForemanOnce(t: Task) {
  showDeliveryForemanToast("Delivery Foreman: analyzing completed run…");
  const prompt = await buildDeliveryForemanPrompt(t);
  chat.value = await sendChatAndReload(prompt, { sendChat, fetchChat });
  if (selectedSessionKey.value === sessionKeyForTask(t)) {
    await refreshAcceptance();
  }
  showDeliveryForemanToast("Delivery Foreman: suggestion ready (open Secretary to view).");
}

async function refreshAcceptance() {
  const key = selectedSessionKey.value.trim();
  if (!key) {
    acceptanceState.value = null;
    acceptanceError.value = "";
    acceptanceLoading.value = false;
    acceptanceExpanded.value = false;
    return;
  }
  acceptanceLoading.value = true;
  acceptanceError.value = "";
  try {
    const res = await fetchAcceptance(key);
    acceptanceState.value = res.ok ? res.state : null;
  } catch (e: any) {
    acceptanceError.value = e?.message ?? String(e);
    acceptanceState.value = null;
  } finally {
    acceptanceLoading.value = false;
  }
}

async function maybeTriggerDeliveryForeman(prev: Task | undefined, next: Task) {
  if (!autoDeliveryForeman.value) return;
  if (!next?.id) return;
  if (!prev) return;
  const runID = next.id;
  if (deliveryForemanSeenRuns.value.has(runID)) return;
  const prevStatus = prev?.status ?? "";
  const nextStatus = next.status ?? "";

  // Only auto-trigger on transitions into terminal states.
  if (isTerminalStatus(prevStatus) || !isTerminalStatus(nextStatus)) return;
  if (!(prevStatus === "running" || prevStatus === "queued" || prevStatus === "")) return;

  persistDeliveryForemanSeen(runID);

  // Non-disruptive: do not auto-open secretary or steal focus.
  if (deliveryForemanRunning.value) {
    deliveryForemanQueue.value = [...deliveryForemanQueue.value, next];
    showDeliveryForemanToast("Delivery Foreman: queued (multiple runs finished).");
    return;
  }
  deliveryForemanRunning.value = true;
  try {
    await runDeliveryForemanOnce(next);
    while (deliveryForemanQueue.value.length) {
      const [head, ...rest] = deliveryForemanQueue.value;
      deliveryForemanQueue.value = rest;
      if (head?.id) await runDeliveryForemanOnce(head);
    }
  } catch (e: any) {
    showDeliveryForemanToast(`Delivery Foreman failed: ${e?.message ?? String(e)}`);
  } finally {
    deliveryForemanRunning.value = false;
  }
}

const ATTENTION_AUTOPILOT_COOLDOWN_MS = 5 * 60 * 1000;

function persistAttentionAutopilotSeen() {
  saveStringMap(LS_KEY_ATTENTION_AUTOPILOT_SEEN, attentionAutopilotSeen.value);
}

function markAttentionAutopilotSeen(sessionKey: string) {
  attentionAutopilotSeen.value = attentionAutopilotMarkSeen(
    attentionAutopilotSeen.value,
    sessionKey,
    Date.now(),
  );
  persistAttentionAutopilotSeen();
}

function stopAttentionAutopilotForSession(sessionKey: string) {
  attentionAutopilotSeen.value = attentionAutopilotStopForSession(
    attentionAutopilotSeen.value,
    sessionKey,
  );
  persistAttentionAutopilotSeen();
}

function enqueueAttentionAutopilot(sessionKey: string) {
  const k = String(sessionKey ?? "").trim();
  if (!k) return;
  if (attentionAutopilotQueued.has(k)) return;
  attentionAutopilotQueued.add(k);
  attentionAutopilotQueue.value = [...attentionAutopilotQueue.value, k].slice(0, 20);
  void runAttentionAutopilotLoop();
}

async function runAttentionAutopilotLoop() {
  if (attentionAutopilotRunning.value) return;
  if (!attentionAutopilotEnabled.value) return;
  attentionAutopilotRunning.value = true;
  try {
    while (attentionAutopilotQueue.value.length) {
      if (!attentionAutopilotEnabled.value) return;
      const [key, ...rest] = attentionAutopilotQueue.value;
      attentionAutopilotQueue.value = rest;
      attentionAutopilotQueued.delete(key);

      const sess = sessionsAll.value.find((s) => s.key === key) ?? null;
      if (!sess) continue;

      if (sess.deleted_at) continue;
      if (!sess.session_id) continue;
      if (sess.status !== "interrupted") continue;
      if (sess.latest.status === "running" || sess.latest.status === "queued") continue;

      const now = Date.now();
      const last = attentionAutopilotSeenAtMs(attentionAutopilotSeen.value, key);
      const should = attentionAutopilotShouldAttempt({
        enabled: attentionAutopilotEnabled.value,
        deleted: Boolean(sess.deleted_at),
        hasSessionID: Boolean(sess.session_id?.trim()),
        sessionStatus: sess.status,
        latestStatus: sess.latest.status,
        nowMs: now,
        lastAttemptMs: last,
        cooldownMs: ATTENTION_AUTOPILOT_COOLDOWN_MS,
      });
      if (!should) continue;

      markAttentionAutopilotSeen(key);

      const short = (sess.session_id || sess.latest.id).slice(0, 8);
      attentionAutopilotNote.value = `Autopilot: resuming ${short}…`;
      try {
        const driver = toolDriverForWorkerType(sess.worker_type);
        const intent = normalizeTaskIntent(sess.latest.task_intent ?? "code");
        const preset = effectiveSafetyPresetForTask(driver, sess.latest);
        if (isHighRiskPreset(driver, preset)) {
          attentionAutopilotNote.value = `Autopilot skipped for ${short}: high-risk preset requires manual resume.`;
          continue;
        }
        const safety = buildRunSafetyPayload(driver, intent, preset);
        const nt = await resumeTaskWithOptions(sess.latest.id, { prompt: "continue", ...safety });
        resumeOriginByRunID.set(nt.id, "autopilot");
        upsertTask(nt);
        attentionAutopilotNote.value = `Autopilot: resume started for ${short}.`;
      } catch (e: any) {
        const msg = e?.message ?? String(e);
        if (attentionAutopilotIsNoConversationFound(msg)) {
          stopAttentionAutopilotForSession(key);
          attentionAutopilotNote.value = `Autopilot 已停止：${short} 在 Claude 侧已不存在。建议：新建会话继续。`;
        } else {
          attentionAutopilotNote.value = `Autopilot: resume failed for ${short}: ${msg}`;
        }
      }
    }
  } finally {
    attentionAutopilotRunning.value = false;
  }
}

function maybeTriggerAttentionAutopilot(prev: Task | undefined, next: Task) {
  if (!attentionAutopilotEnabled.value) return;
  if (!next?.id) return;
  if (!prev) return;
  if (prev.status === next.status) return;
  if (next.status !== "interrupted") return;

  // Non-disruptive: enqueue and run in background; do not steal focus.
  const key = sessionKeyForTask(next);
  enqueueAttentionAutopilot(key);
}

async function onCancelTask() {
  if (!selectedTaskId.value) return;
  errorBanner.value = "";
  try {
    await cancelTask(selectedTaskId.value);
  } catch (e: any) {
    const msg = e?.message ?? String(e);
    if (attentionAutopilotIsNoConversationFound(msg)) {
      stopAttentionAutopilotForSession(sessionKeyForTask(sess.latest));
      errorBanner.value =
        "Resume 失败：Claude 找不到该 session（No conversation found）。建议：直接 New Run 重新开始；或检查 Claude Code 会话是否被清理/禁用持久化。原始错误：" +
        msg;
      return;
    }
    errorBanner.value = msg;
  }
}

async function secretaryCancelSessionRun(s: SessionGroup) {
  if (!s?.latest?.id) return;
  errorBanner.value = "";
  try {
    await cancelTask(s.latest.id);
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

async function secretaryResumeSessionRun(s: SessionGroup) {
  if (!s?.latest?.id) return;
  if (s.deleted_at) {
    errorBanner.value = "该 session 已删除（软删除），无法 resume。";
    return;
  }
  if (!s.session_id) {
    errorBanner.value = "该 session 还没有 session_id，无法 resume。";
    return;
  }
  if (s.latest.status === "running" || s.latest.status === "queued") {
    errorBanner.value = "该 session 仍在运行中，暂不需要 resume。";
    return;
  }
  errorBanner.value = "";
  try {
    const driver = toolDriverForWorkerType(s.worker_type);
    const intent = normalizeTaskIntent(runSafetyIntentByTool.value[s.worker_type] ?? s.latest.task_intent ?? "code");
    const savedPreset = runSafetyPresetByTool.value[s.worker_type] ?? "";
    const preset = normalizeSafetyPreset(driver, intent, savedPreset || effectiveSafetyPresetForTask(driver, s.latest));
    if (isHighRiskPreset(driver, preset)) {
      const ok = await requestHighRiskConfirm({
        title: "高风险确认",
        message: "该 resume 需要高风险设置（权限更大）。继续吗？",
        detail: highRiskPresetSummary(driver, preset),
        confirmLabel: "继续 Resume（我已知晓）",
      });
      if (!ok) return;
    }
    const safety = buildRunSafetyPayload(driver, intent, preset);
    const nt = await resumeTaskWithOptions(s.latest.id, { prompt: "continue", ...safety });
    resumeOriginByRunID.set(nt.id, "manual");
    upsertTask(nt);
    selectedTaskId.value = nt.id;
    await loadLogs(nt.id);
    outputTab.value = "logs";
    closeSecretary();
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

async function onResumeTask() {
  const sess = selectedSession.value;
  if (!sess) return;
  if (sess.deleted_at) {
    errorBanner.value = "该 session 已删除（软删除），无法 resume。";
    return;
  }
  if (!sess.session_id) {
    errorBanner.value = "该 session 还没有 session_id，无法 resume。";
    return;
  }
  if (!resumePrompt.value.trim()) return;
  errorBanner.value = "";
  try {
    const driver = resumeDriver.value;
    const useAutopilot = runSafetyAutopilotEnabled.value && !resumeSafetyOverride.value;

    let payload: RunSafetyPayload = {};
    if (useAutopilot) {
      const preset = effectiveSafetyPresetForTask(driver, sess.latest);
      if (isHighRiskPreset(driver, preset) && !isHighRiskAllowedByInstallUnlock(driver, preset)) {
        const ok = await requestHighRiskConfirm({
          title: "需要解锁安装/下载",
          message: "这个任务需要允许下载/安装（风险更高）。点一次「继续」即可放行。",
          detail: highRiskPresetSummary(driver, preset),
          confirmLabel: "继续（解锁并运行）",
        });
        if (!ok) return;
        runSafetyInstallUnlock.value = true;
      }
      payload = buildSafetyEnvelopePayload();
    } else {
      const intent = resumeTaskIntent.value;
      const preset = resumeSafetyPreset.value;
      if (
        (driver === "claude-code" || driver === "codex") &&
        isHighRiskPreset(driver, preset) &&
        !resumeHighRiskOptIn.value
      ) {
        const ok = await requestHighRiskConfirm({
          title: "高风险确认",
          message: "该 resume 需要高风险设置（权限更大）。继续吗？",
          detail: highRiskPresetSummary(driver, preset),
          confirmLabel: "继续 Resume（我已知晓）",
        });
        if (!ok) return;
        resumeHighRiskOptIn.value = true;
      }

      setStringMapKey(runSafetyIntentByTool, sess.worker_type, intent);
      setStringMapKey(runSafetyPresetByTool, sess.worker_type, preset);

      const envelope = buildSafetyEnvelopePayload();
      payload = { ...envelope, ...buildRunSafetyPayload(driver, intent, preset) };
    }

    const nt = await resumeTaskWithOptions(sess.latest.id, { prompt: resumePrompt.value, ...payload });
    resumeOriginByRunID.set(nt.id, "manual");
    upsertTask(nt);
    selectedTaskId.value = nt.id;
    resumePrompt.value = "";
    await loadLogs(nt.id);
  } catch (e: any) {
    errorBanner.value = e?.message ?? String(e);
  }
}

function isImeComposing(e: KeyboardEvent): boolean {
  // isComposing is reliable on modern browsers; keyCode=229 is a common fallback.
  return Boolean((e as any).isComposing) || (e as any).keyCode === 229;
}

async function onResumeEnter(e: KeyboardEvent) {
  if (isImeComposing(e)) return;
  e.preventDefault();
  await onResumeTask();
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

function resetFilesEditor() {
  filesSelectedPath.value = "";
  filesSelectedKind.value = "";
  filesView.value = "preview";
  filesFileSize.value = 0;
  filesFileTruncated.value = false;
  filesFileContent.value = "";
  filesFileOriginal.value = "";
  filesFileLoading.value = false;
  filesFileError.value = "";
  filesSaving.value = false;
}

function resetFilesState() {
  filesBase.value = "";
  filesRoot.value = null;
  filesLoading.value = false;
  filesError.value = "";
  filesNotice.value = "";
  resetFilesEditor();
}

function closeFiles() {
  if (filesDirty.value && !window.confirm("Discard unsaved changes?")) return;
  filesOpen.value = false;
  resetFilesState();
}

function fsEntryToNode(entry: FSEntry, parentPath: string): FileNode {
  return {
    name: entry.name,
    path: entry.path,
    kind: entry.kind,
    size: typeof entry.size === "number" ? entry.size : undefined,
    parentPath,
    expanded: false,
    loading: false,
    children: [],
  };
}

function findFilesNode(root: FileNode | null, path: string): FileNode | null {
  if (!root) return null;
  if (root.path === path) return root;
  for (const c of root.children) {
    const found = findFilesNode(c, path);
    if (found) return found;
  }
  return null;
}

async function loadFilesDir(node: FileNode, force = false) {
  if (node.kind !== "dir") return;
  if (!force && node.children.length) return;
  node.loading = true;
  filesError.value = "";
  try {
    const res = await fetchFSEntries(node.path);
    node.children = (res.entries ?? []).map((e) => fsEntryToNode(e, node.path));
    node.expanded = true;
  } catch (e: any) {
    filesError.value = e?.message ?? String(e);
    node.expanded = false;
  } finally {
    node.loading = false;
  }
}

async function refreshFilesDir(path: string) {
  const root = filesRoot.value;
  if (!root) return;
  const node = findFilesNode(root, path);
  if (!node || node.kind !== "dir") return;
  await loadFilesDir(node, true);
}

async function toggleFilesDir(node: FileNode) {
  if (node.kind !== "dir") return;
  if (node.expanded) {
    node.expanded = false;
    return;
  }
  node.expanded = true;
  if (!node.children.length) await loadFilesDir(node);
}

function joinPath(dir: string, name: string): string {
  const base = (dir ?? "").trim().replaceAll("\\", "/").replace(/\/+$/, "");
  const next = (name ?? "").trim().replaceAll("\\", "/").replace(/^\/+/, "");
  if (!base) return next;
  if (!next) return base;
  return `${base}/${next}`;
}

function targetDirForFilesOps(): string {
  const root = filesRoot.value;
  if (!root) return "";
  if (filesSelectedKind.value === "dir" && filesSelectedPath.value) {
    return filesSelectedPath.value;
  }
  if (filesSelectedKind.value === "file" && filesSelectedPath.value) {
    return dirnameForBase(filesSelectedPath.value);
  }
  return root.path;
}

async function openFilesFile(path: string, base?: string) {
  const p = (path ?? "").trim();
  if (!p) return;
  filesFileLoading.value = true;
  filesFileError.value = "";
  filesNotice.value = "";
  try {
    const res = await fetchFSRead(p, base);
    filesSelectedPath.value = res.path || p;
    filesSelectedKind.value = "file";
    filesFileSize.value = res.size ?? 0;
    filesFileTruncated.value = Boolean(res.truncated);
    filesFileContent.value = res.content ?? "";
    filesFileOriginal.value = res.content ?? "";
    filesView.value = "preview";
    if (filesFileTruncated.value) {
      filesNotice.value = "File truncated (edit disabled to avoid data loss).";
    }
  } catch (e: any) {
    filesFileError.value = e?.message ?? String(e);
  } finally {
    filesFileLoading.value = false;
  }
}

async function onFilesNodeClick(node: FileNode) {
  if (!node) return;
  if (
    filesDirty.value &&
    filesSelectedKind.value === "file" &&
    node.path !== filesSelectedPath.value &&
    !window.confirm("Discard unsaved changes?")
  ) {
    return;
  }

  filesNotice.value = "";
  filesFileError.value = "";

  if (node.kind === "dir") {
    filesSelectedPath.value = node.path;
    filesSelectedKind.value = "dir";
    filesFileSize.value = 0;
    filesFileTruncated.value = false;
    filesFileContent.value = "";
    filesFileOriginal.value = "";
    filesView.value = "preview";
    await toggleFilesDir(node);
    return;
  }

  await openFilesFile(node.path);
}

function openWorkspaceFilesInNewTab() {
  const sess = selectedSession.value;
  if (!sess) return;
  const base = (sess.workdir ?? "").trim() || ".";
  openInNewTab(`/files?base=${encodePathForQueryValue(base)}`);
}

async function filesSave() {
  if (filesSaving.value) return;
  if (filesSelectedKind.value !== "file") return;
  if (!filesSelectedPath.value) return;
  if (!filesDirty.value) return;
  if (filesFileTruncated.value) {
    filesNotice.value = "Cannot save: file was truncated during read.";
    return;
  }
  filesSaving.value = true;
  filesFileError.value = "";
  filesNotice.value = "";
  try {
    await fsWrite({ path: filesSelectedPath.value, content: filesFileContent.value });
    filesFileOriginal.value = filesFileContent.value;
    filesNotice.value = "Saved.";
    await refreshFilesDir(dirnameForBase(filesSelectedPath.value));
  } catch (e: any) {
    filesFileError.value = e?.message ?? String(e);
  } finally {
    filesSaving.value = false;
  }
}

async function filesNewFile() {
  const root = filesRoot.value;
  if (!root) return;
  if (filesDirty.value && !window.confirm("Discard unsaved changes?")) return;
  const dir = targetDirForFilesOps();
  const name = (window.prompt("New file name") ?? "").trim();
  if (!name) return;
  const path = joinPath(dir, name);

  const parent = findFilesNode(root, dir);
  if (parent && parent.kind === "dir") {
    const exists = parent.children.some((c) => c.name === name);
    if (exists && !window.confirm("File exists. Overwrite?")) return;
  }

  filesNotice.value = "";
  filesError.value = "";
  try {
    await fsWrite({ path, content: "" });
    await refreshFilesDir(dir);
    await openFilesFile(path);
    filesNotice.value = "Created.";
  } catch (e: any) {
    filesError.value = e?.message ?? String(e);
  }
}

async function filesNewFolder() {
  const root = filesRoot.value;
  if (!root) return;
  const dir = targetDirForFilesOps();
  const name = (window.prompt("New folder name") ?? "").trim();
  if (!name) return;
  const path = joinPath(dir, name);

  filesNotice.value = "";
  filesError.value = "";
  try {
    await fsMkdir({ path, recursive: true });
    await refreshFilesDir(dir);
    filesNotice.value = "Created.";
  } catch (e: any) {
    filesError.value = e?.message ?? String(e);
  }
}

async function filesDeleteSelected() {
  const root = filesRoot.value;
  if (!root) return;
  const path = filesSelectedPath.value;
  const kind = filesSelectedKind.value;
  if (!path || !kind) return;
  if (path === root.path) return;
  if (filesDirty.value && !window.confirm("Discard unsaved changes?")) return;

  const label = kind === "dir" ? "folder" : "file";
  const recursive = kind === "dir";
  const ok = window.confirm(
    `Delete ${label}?\n${path}${recursive ? "\n(recursive)" : ""}`,
  );
  if (!ok) return;

  filesError.value = "";
  filesNotice.value = "";
  try {
    await fsDelete({ path, recursive });
    filesNotice.value = "Deleted.";
    const parent = dirnameForBase(path);
    await refreshFilesDir(parent);
    filesSelectedPath.value = parent;
    filesSelectedKind.value = "dir";
    filesFileSize.value = 0;
    filesFileTruncated.value = false;
    filesFileContent.value = "";
    filesFileOriginal.value = "";
    filesView.value = "preview";
  } catch (e: any) {
    filesError.value = e?.message ?? String(e);
  }
}

function pinnedWorkspaceName(path: string): string {
  const key = normalizePathForCompare(path);
  if (!key) return "";
  return (pinnedWorkspaceNames.value[key] ?? "").trim();
}

function workspaceOptionLabel(path: string): string {
  const name = pinnedWorkspaceName(path);
  if (!name) return path;
  return `${name} · ${path}`;
}

function openWorkspaceRename(path: string) {
  const p = path.trim();
  if (!p) return;
  workspaceRenamePath.value = p;
  workspaceRenameValue.value = pinnedWorkspaceName(p);
  workspaceRenameOpen.value = true;
  void nextTick(() => workspaceRenameInputEl.value?.focus());
}

function closeWorkspaceRename() {
  workspaceRenameOpen.value = false;
}

function saveWorkspaceRename() {
  const p = workspaceRenamePath.value.trim();
  if (!p) return;
  const key = normalizePathForCompare(p);
  if (!key) return;
  const name = workspaceRenameValue.value.trim();
  const next = { ...pinnedWorkspaceNames.value };
  if (name) next[key] = name;
  else delete next[key];
  pinnedWorkspaceNames.value = next;
  workspaceRenameOpen.value = false;
}

function setWorkspace(path: string) {
  workspaceSelect.value = path;
}

function clearWorkspace() {
  workspaceSelect.value = "";
}

function toggleWorkspaceFilter(path: string) {
  const p = path.trim();
  if (!p) return;
  const key = normalizePathForCompare(p);
  const next = workspaceFilters.value.slice();
  const idx = next.findIndex((x) => normalizePathForCompare(x) === key);
  if (idx >= 0) next.splice(idx, 1);
  else next.unshift(p);
  workspaceFilters.value = next.slice(0, 6);
  if (workspaceFilters.value.length <= 1) {
    workspaceSelect.value = workspaceFilters.value[0] ?? "";
  } else {
    // For multiple, clear the dropdown to avoid misleading single selection.
    workspaceSelect.value = "";
  }
}

function removeWorkspaceFilter(path: string) {
  const key = normalizePathForCompare(path);
  workspaceFilters.value = workspaceFilters.value.filter(
    (x) => normalizePathForCompare(x) !== key,
  );
  if (workspaceFilters.value.length <= 1) {
    workspaceSelect.value = workspaceFilters.value[0] ?? "";
  }
}

function addWorkspaceFilter(path: string) {
  const p = path.trim();
  if (!p) return;
  const key = normalizePathForCompare(p);
  const next = workspaceFilters.value.slice();
  if (next.some((x) => normalizePathForCompare(x) === key)) return;
  next.unshift(p);
  workspaceFilters.value = next.slice(0, 6);
  workspaceSelect.value = "";
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
  if (workspaceFilters.value.some((x) => normalizePathForCompare(x) === key)) {
    removeWorkspaceFilter(path);
  }
}

function openAuthSettings() {
  authSettingsError.value = "";
  authSettingsOpen.value = true;
  refreshAuth();
}

async function refreshTools() {
  toolsError.value = "";
  toolsLoading.value = true;
  try {
    const res = await fetchTools();
    toolsList.value = res.tools ?? [];
    if (!toolsSelectedID.value && toolsList.value.length) {
      toolsSelectedID.value = toolsList.value[0].id;
    }
    if (!newWorkerType.value && toolsList.value.length) {
      newWorkerType.value = toolsList.value[0].id;
    }
    if (newWorkerType.value && toolsList.value.length) {
      const ok = toolsList.value.some((t) => t.id === newWorkerType.value);
      if (!ok) newWorkerType.value = toolsList.value[0].id;
    }
  } catch (e: any) {
    toolsError.value = e?.message ?? String(e);
  } finally {
    toolsLoading.value = false;
  }
}

function toolForID(id: string): Tool | null {
  const v = String(id ?? "").trim();
  if (!v) return null;
  return toolsList.value.find((t) => t.id === v) ?? null;
}

const newTool = computed(() => toolForID(newWorkerType.value));

function toolDriverForWorkerType(workerType: string): ToolDriver {
  const id = String(workerType ?? "").trim();
  const t = toolForID(id);
  if (t?.driver) return t.driver;
  if (id === "claude-code") return "claude-code";
  if (id === "codex") return "codex";
  return "exec";
}

function isHighRiskAllowedByInstallUnlock(driver: ToolDriver, preset: string): boolean {
  if (!runSafetyInstallUnlock.value) return false;
  const p = String(preset ?? "").trim();
  if (driver === "codex") return p === "danger-full-access";
  if (driver === "claude-code") return p === "unsafe";
  return false;
}

function setStringMapKey(map: { value: Record<string, string> }, key: string, value: string) {
  const k = String(key ?? "").trim();
  if (!k) return;
  const v = String(value ?? "").trim();
  const next = { ...map.value };
  if (v) next[k] = v;
  else delete next[k];
  map.value = next;
}

function buildSafetyEnvelopePayload(): Pick<RunSafetyPayload, "safety_envelope"> {
  return runSafetyInstallUnlock.value ? { safety_envelope: "install-enabled" } : {};
}

const newRunDriver = computed<ToolDriver>(() => toolDriverForWorkerType(newWorkerType.value));
const newRunTaskIntent = computed<TaskIntent>({
  get: () => normalizeTaskIntent(runSafetyIntentByTool.value[newWorkerType.value] ?? "code"),
  set: (value) => setStringMapKey(runSafetyIntentByTool, newWorkerType.value, value),
});
const newRunSafetyPreset = computed<string>({
  get: () =>
    normalizeSafetyPreset(
      newRunDriver.value,
      newRunTaskIntent.value,
      runSafetyPresetByTool.value[newWorkerType.value] ?? "",
    ),
  set: (value) => setStringMapKey(runSafetyPresetByTool, newWorkerType.value, value),
});

const newRunUseAutopilot = computed<boolean>(
  () => runSafetyAutopilotEnabled.value && !newRunSafetyOverride.value,
);
const newRunShowManualSafety = computed<boolean>(() => !newRunUseAutopilot.value);

watch([newWorkerType, newRunTaskIntent, newRunSafetyPreset], () => {
  newRunHighRiskOptIn.value = false;
});

function formatArgsForEdit(args?: string[]) {
  return (args ?? []).join(" ");
}

function formatEnvForEdit(env?: Record<string, string>) {
  const entries = Object.entries(env ?? {}).sort((a, b) => a[0].localeCompare(b[0]));
  return entries.map(([k, v]) => `${k}=${v}`).join("\n");
}

function loadToolIntoEditor(t: Tool) {
  toolsSelectedID.value = t.id;
  toolEditID.value = t.id;
  toolEditDriver.value = t.driver;
  toolEditCommand.value = t.command;
  toolEditArgs.value = formatArgsForEdit(t.args);
  toolEditEnv.value = formatEnvForEdit(t.env);
}

function openToolsSettings() {
  toolsError.value = "";
  toolsSettingsOpen.value = true;
  if (!toolsList.value.length) void refreshTools();
  const selected = toolForID(toolsSelectedID.value) ?? toolsList.value[0] ?? null;
  if (selected) loadToolIntoEditor(selected);
}

function startNewTool() {
  toolsSelectedID.value = "";
  toolEditID.value = "";
  toolEditDriver.value = "claude-code";
  toolEditCommand.value = "";
  toolEditArgs.value = "";
  toolEditEnv.value = "";
}

function parseArgsText(s: string): string[] {
  const parts = String(s ?? "")
    .trim()
    .split(/\s+/)
    .map((x) => x.trim())
    .filter(Boolean);
  return parts;
}

function parseEnvText(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  const lines = String(s ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
  for (const line of lines) {
    const idx = line.indexOf("=");
    if (idx <= 0) continue;
    const k = line.slice(0, idx).trim();
    const v = line.slice(idx + 1).trim();
    if (!k) continue;
    out[k] = v;
  }
  return out;
}

async function saveTool() {
  toolsError.value = "";
  toolsSaving.value = true;
  try {
    const tool: Tool = {
      id: toolEditID.value.trim(),
      driver: toolEditDriver.value,
      command: toolEditCommand.value.trim(),
      args: parseArgsText(toolEditArgs.value),
      env: parseEnvText(toolEditEnv.value),
    };
    await upsertTool({ tool });
    await refreshTools();
    const saved = toolForID(tool.id);
    if (saved) loadToolIntoEditor(saved);
  } catch (e: any) {
    toolsError.value = e?.message ?? String(e);
  } finally {
    toolsSaving.value = false;
  }
}

async function deleteToolOverride() {
  const id = toolEditID.value.trim();
  if (!id) return;
  toolsError.value = "";
  toolsSaving.value = true;
  try {
    await deleteTool({ id });
    await refreshTools();
    const next = toolForID("claude-code") ?? toolsList.value[0] ?? null;
    if (next) loadToolIntoEditor(next);
  } catch (e: any) {
    toolsError.value = e?.message ?? String(e);
  } finally {
    toolsSaving.value = false;
  }
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
  const driver =
    newTool.value?.driver ??
    (newWorkerType.value === "claude-code" || newWorkerType.value === "codex"
      ? (newWorkerType.value as ToolDriver)
      : "");
  if (driver === "claude-code" && !st.claude.available) {
    return "claude-code 未检测到可用鉴权：请设置 ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN，或在终端运行一次 `claude /login`。";
  }
  if (driver === "codex" && !st.codex.available) {
    return "codex 未检测到可用鉴权：请设置 OPENAI_API_KEY。";
  }
  return "";
});

onMounted(async () => {
  await refresh();
  if (selectedTaskId.value) await loadLogs(selectedTaskId.value);
  await refreshAuth();
  await refreshTools();
  connectEvents();
  window.addEventListener("keydown", onGlobalKeyDown);
  window.addEventListener("popstate", onRoutePopState);
  document.addEventListener("mousedown", onSessionActionsMenuDocumentMouseDown, true);
  document.addEventListener("mousedown", onHeaderMoreDocumentMouseDown, true);
  window.addEventListener("keydown", onSessionActionsMenuKeyDown, true);
  window.addEventListener("resize", closeSessionActionsMenu, true);
  window.addEventListener("scroll", closeSessionActionsMenu, true);
  focusInHandler = (e: FocusEvent) => {
    if (!isPhone.value) return;
    const t = e.target as any;
    if (!t) return;
    const tag = String(t.tagName ?? "").toLowerCase();
    if (tag !== "input" && tag !== "textarea" && tag !== "select") return;
    scrollFocusedIntoView(t as HTMLElement);
  };
  window.addEventListener("focusin", focusInHandler);

  phoneMq = window.matchMedia("(max-width: 900px)");
  phoneMqHandler = () => {
    if (!phoneMq) return;
    isPhone.value = phoneMq.matches;
    if (isPhone.value) {
      sessionsDrawerOpen.value = false;
      sessionsFiltersOpen.value = false;
    } else {
      sessionsDrawerOpen.value = false;
    }
  };
  phoneMqHandler();
  phoneMq.addEventListener?.("change", phoneMqHandler);

  applyRouteFromLocation();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onGlobalKeyDown);
  window.removeEventListener("popstate", onRoutePopState);
  document.removeEventListener("mousedown", onSessionActionsMenuDocumentMouseDown, true);
  document.removeEventListener("mousedown", onHeaderMoreDocumentMouseDown, true);
  window.removeEventListener("keydown", onSessionActionsMenuKeyDown, true);
  window.removeEventListener("resize", closeSessionActionsMenu, true);
  window.removeEventListener("scroll", closeSessionActionsMenu, true);
  if (focusInHandler) window.removeEventListener("focusin", focusInHandler);
  if (phoneMq && phoneMqHandler) {
    phoneMq.removeEventListener?.("change", phoneMqHandler);
  }
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
    let title = "";
    let deletedAt = "";
    for (const r of runs) {
      score = Math.max(score, r.score);
      stderrCount = Math.max(stderrCount, r.stderr_count);
      if (!warning && r.warning) warning = r.warning;
      if (!title && (r.session_title ?? "").trim()) title = (r.session_title ?? "").trim();
      if (!deletedAt && (r.session_deleted_at ?? "").trim()) deletedAt = (r.session_deleted_at ?? "").trim();
    }

    out.push({
      key,
      session_id: latest.session_id?.trim() ?? "",
      title,
      deleted_at: deletedAt,
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
  const roots = workspaceFilters.value.map((x) => x.trim()).filter(Boolean);
  const needle = sessionSearch.value.trim().toLowerCase();
  return sessionsAll.value.filter((s) => {
    if (roots.length > 0 && !roots.some((r) => isWithinWorkspace(r, s.workdir)))
      return false;
    if (!needle) return true;

    const sid = (s.session_id || s.latest.id).toLowerCase();
    const title = (s.title ?? "").toLowerCase();
    const prompt = (s.latest.prompt ?? "").toLowerCase();
    const workdir = (s.workdir ?? "").toLowerCase();
    return (
      sid.includes(needle) ||
      title.includes(needle) ||
      prompt.includes(needle) ||
      workdir.includes(needle) ||
      s.worker_type.toLowerCase().includes(needle) ||
      s.status.toLowerCase().includes(needle)
    );
  });
});

const pagedSessions = computed(() => {
  return filteredSessions.value.slice(0, sessionsLimit.value);
});

const canLoadMoreSessions = computed(() => {
  return pagedSessions.value.length < filteredSessions.value.length;
});

function loadMoreSessions() {
  sessionsLimit.value = Math.min(
    filteredSessions.value.length,
    sessionsLimit.value + 40,
  );
}

const selectedSession = computed(() => {
  const key = selectedSessionKey.value;
  if (!key) return null;
  return sessionsAll.value.find((s) => s.key === key) ?? null;
});

const resumeDriver = computed<ToolDriver>(() =>
  toolDriverForWorkerType(selectedSession.value?.worker_type ?? ""),
);
const resumeTaskIntent = computed<TaskIntent>({
  get: () => {
    const sess = selectedSession.value;
    if (!sess) return "code";
    const raw = runSafetyIntentByTool.value[sess.worker_type] ?? sess.latest.task_intent ?? "code";
    return normalizeTaskIntent(raw);
  },
  set: (value) => {
    const sess = selectedSession.value;
    if (!sess) return;
    setStringMapKey(runSafetyIntentByTool, sess.worker_type, value);
  },
});
const resumeSafetyPreset = computed<string>({
  get: () => {
    const sess = selectedSession.value;
    if (!sess) return "";
    const raw = runSafetyPresetByTool.value[sess.worker_type] ?? sess.latest.safety_preset ?? "";
    return normalizeSafetyPreset(resumeDriver.value, resumeTaskIntent.value, raw);
  },
  set: (value) => {
    const sess = selectedSession.value;
    if (!sess) return;
    setStringMapKey(runSafetyPresetByTool, sess.worker_type, value);
  },
});

const resumeUseAutopilot = computed<boolean>(
  () => runSafetyAutopilotEnabled.value && !resumeSafetyOverride.value,
);
const resumeShowManualSafety = computed<boolean>(() => !resumeUseAutopilot.value);
const resumeAutopilotHighRiskBlocked = computed<boolean>(() => {
  if (!resumeUseAutopilot.value) return false;
  const sess = selectedSession.value;
  if (!sess) return false;
  const driver = resumeDriver.value;
  const preset = effectiveSafetyPresetForTask(driver, sess.latest);
  if (!isHighRiskPreset(driver, preset)) return false;
  return !isHighRiskAllowedByInstallUnlock(driver, preset);
});

watch([selectedSessionKey, resumeTaskIntent, resumeSafetyPreset], () => {
  resumeHighRiskOptIn.value = false;
});

watch(
  selectedSessionKey,
  () => {
    // Default to autopilot on session switches; advanced overrides are per-run.
    resumeSafetyOverride.value = false;
    resumeHighRiskOptIn.value = false;
    void refreshAcceptance();
  },
  { immediate: true },
);

let acceptancePollTimer: number | null = null;
watch(
  [selectedSessionKey, () => acceptanceState.value?.status ?? ""],
  ([key, status]) => {
    if (acceptancePollTimer != null) {
      window.clearInterval(acceptancePollTimer);
      acceptancePollTimer = null;
    }
    if (!key) return;
    if (String(status).toLowerCase() !== "running") return;
    acceptancePollTimer = window.setInterval(() => {
      void refreshAcceptance();
    }, 4000);
  },
  { immediate: true },
);

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

const secretarySessionsAll = computed(() => {
  const scope = secretaryScope.value;
  if (scope === "all") return sessionsAll.value;
  const filters = workspaceFilters.value;
  if (!filters.length) return sessionsAll.value;
  return sessionsAll.value.filter((s) =>
    filters.some((w) => isWithinWorkspace(w, s.workdir)),
  );
});

const secretaryCounts = computed(() => {
  const sess = secretarySessionsAll.value;
  const out: Record<string, number> = {
    total: sess.length,
    running: 0,
    queued: 0,
    blocked: 0,
    failed: 0,
    interrupted: 0,
    succeeded: 0,
    canceled: 0,
  };
  for (const s of sess) {
    out[s.status] = (out[s.status] ?? 0) + 1;
  }
  return out;
});

const anyRunning = computed(() =>
  Array.from(tasks.value.values()).some((t) => t.status === "running"),
);

const needsAttentionSessions = computed(() => {
  return secretarySessionsAll.value
    .filter(
      (s) =>
        s.status !== "succeeded" &&
        (s.score > 0 ||
          s.status === "failed" ||
          s.status === "blocked" ||
          s.status === "interrupted"),
    )
    .slice(0, 6);
});

const secretaryBriefing = computed(() => {
  const c = secretaryCounts.value;
  if (c.total === 0) return "当前还没有 session。";

  const lines: string[] = [];
  if (secretaryScope.value === "all") {
    lines.push("scope: all");
  } else if (workspaceFilters.value.length) {
    lines.push(`scope: current · workspaces ${workspaceFilters.value.length}`);
  } else {
    lines.push("scope: current · no workspace filters");
  }
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
    if (filePreviewOpen.value) {
      closeFilePreview();
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
    if (runsOpen.value) {
      runsOpen.value = false;
      return;
    }
    if (filesOpen.value) {
      closeFilesPage();
      return;
    }
    if (skillsOpen.value) {
      closeSkillsPage();
      return;
    }
    if (secretaryOpen.value) {
      closeSecretary();
      return;
    }
    if (liveOpen.value) {
      liveOpen.value = false;
      return;
    }
    if (headerMoreEl.value?.open) {
      closeHeaderMoreMenu();
      return;
    }
    if (sessionsDrawerOpen.value) {
      sessionsDrawerOpen.value = false;
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
  if (e.key === "l" || e.key === "L") {
    e.preventDefault();
    if (liveOpen.value) liveOpen.value = false;
    else openLive();
  }
}

watch(newRunOpen, async (open) => {
  if (!open) return;
  await nextTick();
  newRunPromptEl.value?.focus();
});

function applyMermaidTheme() {
  try {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: theme.value === "dark" ? "dark" : "default",
    });
  } catch {
    // ignore
  }
}

async function renderMermaidIfNeeded() {
  if (outputTab.value !== "result") return;
  if (resultPreviewTab.value !== "markdown") return;
  const html = selectedResultHtml.value;
  if (!html.includes("mermaid")) return;
  await nextTick();
  try {
    const root = document.querySelector(".resultBox");
    if (!root) return;
    const nodes = Array.from(root.querySelectorAll<HTMLElement>(".mermaid"));
    if (nodes.length === 0) return;
    await mermaid.run({ nodes });
  } catch {
    // ignore mermaid parse errors
  }
}

async function renderFilePreviewMermaidIfNeeded() {
  if (!filePreviewOpen.value) return;
  if (filePreviewTab.value !== "preview") return;
  if (!filePreviewIsMarkdown.value) return;
  const html = filePreviewMarkdownHtml.value;
  if (!html.includes("mermaid")) return;
  await nextTick();
  try {
    const root = filePreviewBoxEl.value;
    if (!root) return;
    const nodes = Array.from(root.querySelectorAll<HTMLElement>(".mermaid"));
    if (nodes.length === 0) return;
    await mermaid.run({ nodes });
  } catch {
    // ignore mermaid parse errors
  }
}

watch(
  [theme, outputTab, resultPreviewTab, selectedResultHtml],
  async () => {
    applyMermaidTheme();
    await renderMermaidIfNeeded();
  },
  { immediate: true },
);

watch([theme, filePreviewOpen, filePreviewTab, filePreviewMarkdownHtml], async () => {
  applyMermaidTheme();
  await renderFilePreviewMermaidIfNeeded();
});

let feedCoachTimer: number | null = null;
watch(
  [anyRunning, feedCoachDismissed, secretaryOpen, liveOpen],
  ([running, dismissed, secOpen, isLiveOpen]) => {
    if (dismissed) return;
    if (secOpen || isLiveOpen) {
      feedCoachOpen.value = false;
      return;
    }
    if (!running) return;

    feedCoachOpen.value = true;
    if (feedCoachTimer != null) window.clearTimeout(feedCoachTimer);
    feedCoachTimer = window.setTimeout(() => {
      if (!feedCoachDismissed.value) feedCoachOpen.value = false;
    }, 12_000);
  },
  { immediate: true },
);
</script>

<template>
  <div class="page">
    <header class="header">
      <div class="headerLeft">
        <button
          type="button"
          class="menuBtn"
          @click="sessionsDrawerOpen = !sessionsDrawerOpen"
          :title="sessionsDrawerOpen ? 'Close sessions' : 'Open sessions'"
          :aria-label="sessionsDrawerOpen ? 'Close sessions' : 'Open sessions'"
        >
          <span class="menuIcon" aria-hidden="true">{{
            sessionsDrawerOpen ? "✕" : "≡"
          }}</span>
        </button>
        <button type="button" class="titleBtn" @click="goHome" title="Home">
          ControlCCX
        </button>
      </div>
      <div class="headerRight">
        <div class="sub" v-if="systemInfo">
          {{ systemInfo.os }}/{{ systemInfo.arch }} ·
          {{ systemInfo.hostname }} · Go {{ systemInfo.go_version }}
        </div>
	        <button type="button" class="primary" @click="openNewRun">
	          New Run
	        </button>
          <details ref="headerMoreEl" class="headerMore">
            <summary class="headerMoreBtn" title="More" aria-label="More">⋯</summary>
            <div class="headerMorePopup">
              <button type="button" class="headerMoreItem" @click="onToggleThemeFromMenu">
                {{ theme === "dark" ? "Day" : "Night" }}
              </button>
              <button
                type="button"
                class="headerMoreItem"
                @click="onOpenLiveFromMenu"
                :title="anyRunning ? 'Open Live Feed (L · running)' : 'Open Live Feed (L)'"
              >
                <span v-if="anyRunning" class="liveDot" aria-hidden="true">●</span>
                Live
              </button>
              <button type="button" class="headerMoreItem" @click="onOpenSkillsFromMenu">
                Skills
              </button>
              <button type="button" class="headerMoreItem" @click="onOpenSettingsFromMenu">
                Settings
              </button>
            </div>
          </details>
	      </div>
	    </header>

    <div v-if="errorBanner" class="banner">{{ errorBanner }}</div>

    <div v-if="isPhone && sessionsDrawerOpen" class="sessionsOverlay" @click.self="sessionsDrawerOpen = false"></div>

    <div class="grid" :class="{ gridSingle: !sessionsDrawerOpen }">
	      <section v-if="skillsOpen" class="panel skillsPagePanel">
          <div class="skillsPageWrap">
            <div class="skillsLeftCol">
              <SkillsGovernancePanel />

              <SkillsVersionsPanel
                :loading="skillsVersionsLoading"
                :error="skillsVersionsError"
                :data="skillsVersionsData"
                v-model:new-id="skillsVersionNewId"
                v-model:new-note="skillsVersionNewNote"
                :creating="skillsVersionsCreating"
                :deleting="skillsVersionsDeleting"
                @refresh="refreshSkillVersions"
                @create="createSkillVersionFromForm"
                @delete="deleteSkillVersionByID"
              />
            </div>

            <SkillsPanel
              :loading="skillsLoading"
              :error="skillsError"
              :data="skillsData"
              v-model:filter="skillsFilter"
              v-model:limit="skillsLimit"
              :range-label="skillsRangeLabel"
              :can-prev="skillsCanPrev"
              :can-next="skillsCanNext"
              :action-busy="skillsActionBusy"
              :summarize-target="summarizeSkillTarget"
              :badge-class="skillBadgeClass"
              :make-key="skillsKey"
              @refresh="refreshSkills"
              @prev-page="skillsPrevPage"
              @next-page="skillsNextPage"
              @toggle="onSkillsToggle"
            />
          </div>
	      </section>

      <FilesModal
        v-else-if="filesOpen"
        :root="filesRoot"
        :loading="filesLoading"
        :error="filesError"
        :notice="filesNotice"
        :sidebarWidth="filesSidebarWidth"
        :resizing="filesResizing"
        :visibleNodes="filesVisibleNodes"
        :selectedPath="filesSelectedPath"
        :selectedKind="filesSelectedKind"
        v-model:view="filesView"
        :dirty="filesDirty"
        :fileSize="filesFileSize"
        :fileTruncated="filesFileTruncated"
        v-model:fileContent="filesFileContent"
        :fileError="filesFileError"
        :fileLoading="filesFileLoading"
        :saving="filesSaving"
        :isMarkdown="filesIsMarkdown"
        :previewHtml="filesPreviewHtml"
        :codeHtml="filesCodeHtml"
        :normalizePathForCompare="normalizePathForCompare"
        @back="closeFilesPage"
        @refreshRoot="filesRoot && refreshFilesDir(filesRoot.path)"
        @newFile="filesNewFile"
        @newFolder="filesNewFolder"
        @deleteSelected="filesDeleteSelected"
        @refreshDir="filesRoot && refreshFilesDir(filesRoot.path)"
        @nodeClick="onFilesNodeClick"
        @startResize="startFilesResize"
        @copy="copyText"
        @save="filesSave"
        @markdownClick="onFilesPreviewMarkdownClick"
      />

      <template v-else>
      <section
        v-if="sessionsDrawerOpen"
        class="panel sessionsPanel"
        :class="{ sessionsDrawerPanel: isPhone }"
      >
        <h2>
          Sessions
          <span class="h2Spacer"></span>
          <span class="h2Meta"
            >{{ pagedSessions.length }} / {{ filteredSessions.length }}</span
          >
          <button
            type="button"
            class="h2Btn"
            @click="sessionsDrawerOpen = false"
            aria-label="Close sessions"
          >
            ✕
          </button>
        </h2>
        <div class="list">
          <div class="workspaceBar">
            <div class="workspaceLeft">
              <span class="workspaceTitle">Workspace</span>
              <select v-model="workspaceSelect">
                <option value="">All</option>
                <optgroup v-if="pinnedWorkspaces.length" label="Pinned">
                  <option
                    v-for="p in pinnedWorkspaces"
                    :key="'p-' + p"
                    :value="p"
                  >
                    {{ workspaceOptionLabel(p) }}
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
              @click="sessionsFiltersOpen = !sessionsFiltersOpen"
            >
              {{
                sessionsFiltersOpen
                  ? "Less"
                  : workspaceFilters.length
                    ? `Filters (${workspaceFilters.length})`
                    : "Filters"
              }}
            </button>
            <button
              type="button"
              @click="clearWorkspace"
              :disabled="workspaceFilters.length === 0"
              title="Clear workspace filters"
            >
              All
            </button>
          </div>

          <div v-if="sessionsFiltersOpen" class="filtersBlock">
            <div class="filtersActions">
              <button
                type="button"
                @click="setWorkspace(newWorkdir)"
                :disabled="!newWorkdir.trim()"
              >
                Set Workdir
              </button>
              <button
                type="button"
                @click="addWorkspaceFilter(newWorkdir)"
                :disabled="!newWorkdir.trim()"
                title="Add workdir as an extra workspace filter"
              >
                Add Workdir
              </button>
              <button
                type="button"
                @click="pinWorkspace(workspaceSelect || newWorkdir)"
                :disabled="!(workspaceSelect || newWorkdir).trim()"
              >
                Pin
              </button>
            </div>

              <div v-if="workspaceFilters.length" class="activeFilters">
              <div class="filtersTitle">Active</div>
              <div class="pinnedWorkspaces">
                <div v-for="p in workspaceFilters" :key="'a-' + p" class="pinnedItem">
                  <button
                    type="button"
                    class="pinnedBtn active"
                    @click="toggleWorkspaceFilter(p)"
                    :title="p"
                  >
                    <template v-if="pinnedWorkspaceName(p)">
                      <span class="pinName">{{ pinnedWorkspaceName(p) }}</span>
                      <span class="pinSub mono">{{ p }}</span>
                    </template>
                    <span v-else class="mono">{{ p }}</span>
                  </button>
                  <button
                    type="button"
                    class="pinnedX"
                    @click="removeWorkspaceFilter(p)"
                    title="Remove"
                  >
                    ✕
                  </button>
                </div>
              </div>
            </div>

            <div v-if="pinnedWorkspaces.length" class="activeFilters">
              <div class="filtersTitle">Pinned</div>
              <div class="pinnedWorkspaces">
                <div v-for="p in pinnedWorkspaces" :key="p" class="pinnedItem">
                  <button
                    type="button"
                    class="pinnedBtn"
                    :class="{
                      active: workspaceFilters.some(
                        (x) =>
                          normalizePathForCompare(x) === normalizePathForCompare(p),
                      ),
                    }"
                    @click="toggleWorkspaceFilter(p)"
                    :title="p"
                  >
                    <template v-if="pinnedWorkspaceName(p)">
                      <span class="pinName">{{ pinnedWorkspaceName(p) }}</span>
                      <span class="pinSub mono">{{ p }}</span>
                    </template>
                    <span v-else class="mono">{{ p }}</span>
                  </button>
                  <button
                    type="button"
                    class="pinnedEdit"
                    @click="openWorkspaceRename(p)"
                    title="Rename"
                  >
                    ✎
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
            </div>

            <div class="activeFilters">
              <div class="filtersTitle">Sessions</div>
              <label class="settingsToggleRow">
                <input type="checkbox" v-model="sessionsShowDeleted" />
                <span>Show deleted</span>
              </label>
              <div class="tinyHint">Soft-deleted sessions are hidden by default.</div>
            </div>
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

          <div class="listMeta">
            Showing {{ pagedSessions.length }} / {{ filteredSessions.length }}
            sessions
            <span v-if="workspaceFilters.length">
              · workspaces {{ workspaceFilters.length }}
            </span>
          </div>

          <div
            v-for="s in pagedSessions"
            :key="s.key"
            class="row"
            :class="{ active: s.key === selectedSessionKey, deleted: !!s.deleted_at }"
            :title="s.latest.warning || s.warning || undefined"
            role="button"
            tabindex="0"
            @click="onSelectTask(s.latest.id)"
            @keydown.enter.prevent="onSelectTask(s.latest.id)"
            @keydown.space.prevent="onSelectTask(s.latest.id)"
          >
            <div class="rowTop">
              <div class="rowTopLeft">
                <span class="mono" :title="s.session_id || s.latest.id">{{
                  sessionShortID(s)
                }}</span>
                <span v-if="s.title" class="rowName" :title="s.title">{{
                  s.title
                }}</span>
                <span v-if="s.deleted_at" class="pill deleted">deleted</span>
              </div>
              <div class="rowTopRight">
                <span
                  v-if="s.latest.warning || s.warning"
                  class="warn"
                  :title="s.latest.warning || s.warning"
                  >⚠</span
                >
                <span class="pill" :class="s.status">{{ s.status }}</span>
                <span class="pill kind">{{ s.runs.length }} runs</span>
                <button
                  type="button"
                  class="rowMoreBtn"
                  title="More"
                  aria-label="Session actions"
                  @click.stop="toggleSessionActionsMenu(s, $event)"
                >
                  ⋯
                </button>
              </div>
            </div>
            <div class="rowSub">
              <span class="rowWorkdir mono" :title="s.workdir">{{
                workdirLabelForSession(s.workdir)
              }}</span>
              <span
                v-if="promptSummary(s.latest.prompt)"
                class="rowPrompt"
                :title="s.latest.prompt"
                >{{ promptSummary(s.latest.prompt) }}</span
              >
            </div>
          </div>

          <button
            v-if="canLoadMoreSessions"
            type="button"
            class="loadMore"
            @click="loadMoreSessions"
          >
            Load more
          </button>
        </div>
      </section>

      <section class="panel">
        <div v-if="!selectedSession" class="detail homeStart">
          <div class="homeHero">
            <div class="homeTitle">Start a new run</div>
            <div class="homeSub">
              History is hidden by default. Open Sessions only when you need it.
            </div>
          </div>

          <div v-if="anyRunning" class="homeStatus">
            <div class="homeStatusText">
              <span class="pill running">running</span>
              <span class="tinyHint">There are runs in progress.</span>
            </div>
            <div class="homeStatusActions">
              <button type="button" @click="openLive">Open Live</button>
              <button type="button" @click="sessionsDrawerOpen = true">
                Open Sessions
              </button>
            </div>
          </div>

          <div class="form homeForm">
            <label class="full">
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
                rows="8"
                placeholder="Describe the task to run..."
                @keydown.meta.enter.prevent="runFromHome"
                @keydown.ctrl.enter.prevent="runFromHome"
              ></textarea>
              <div class="tinyHint">Tip: Ctrl/Cmd + Enter to run.</div>
            </label>

            <div class="homeActions full">
              <button
                type="button"
                class="primary"
                @click="runFromHome"
                :disabled="!newPrompt.trim() || highRiskConfirmOpen || homeRunBusy"
              >
                Run
              </button>
              <button type="button" @click="openNewRun">Advanced…</button>
              <button type="button" @click="sessionsDrawerOpen = true">
                Sessions
              </button>
            </div>
          </div>
        </div>
        <div v-else class="detail">
          <div class="detailHeader compact">
            <div class="detailTop">
              <div class="detailTopLeft">
                <span
                  class="mono detailSid"
                  :title="selectedSession.session_id || selectedSession.latest.id"
                  >{{
                    (selectedSession.session_id || selectedSession.latest.id).slice(0, 8)
                  }}</span
                >
                <span
                  v-if="selectedSession.title"
                  class="detailName"
                  :title="selectedSession.title"
                  >{{ selectedSession.title }}</span
                >
                <span v-if="selectedSession.deleted_at" class="pill deleted">deleted</span>
                <span class="pill" :class="selectedSession.status">{{
                  selectedSession.status
                }}</span>
                <span
                  v-if="acceptanceState"
                  class="pill acceptance"
                  :title="`Acceptance ${acceptanceState.status} · ${acceptanceState.current_gate || '(no gate)'} · ${acceptanceState.summary || ''}`"
	                >
	                  Acc {{ acceptanceState.iteration }}/{{ acceptanceState.max_iterations }}
	                </span>
	                <span class="pill kind">{{ selectedSession.worker_type }}</span>
	                <span
	                  v-if="
	                    toolDriverForWorkerType(selectedSession.worker_type) === 'codex' ||
	                    toolDriverForWorkerType(selectedSession.worker_type) === 'claude-code'
	                  "
	                  class="pill safety"
	                  :title="`Safety: ${normalizeTaskIntent(selectedSession.latest.task_intent ?? 'code')}/${effectiveSafetyPresetForTask(toolDriverForWorkerType(selectedSession.worker_type), selectedSession.latest)}`"
	                >
	                  Safety
	                  {{
	                    normalizeTaskIntent(selectedSession.latest.task_intent ?? "code")
	                  }}/{{ effectiveSafetyPresetForTask(toolDriverForWorkerType(selectedSession.worker_type), selectedSession.latest) }}
	                </span>
	                <button
	                  type="button"
	                  class="detailMini detailMiniBtn"
	                  @click="openRuns"
                  title="Open runs"
                >
                  {{ selectedSession.runs.length }} runs
                </button>
                <button
                  type="button"
                  class="detailMini detailMiniBtn"
                  @click="openWorkspaceFilesInNewTab"
                  title="Browse workspace files"
                >
                  Files
                </button>
                <span
                  v-if="selectedTask?.warning || selectedSession.warning"
                  class="warn"
                  :title="selectedTask?.warning || selectedSession.warning"
                  >⚠</span
                >
                <span
                  v-if="selectedTask?.error"
                  class="warn"
                  :title="selectedTask.error"
                  >!</span
                >
                <span
                  v-if="selectedRunActivity"
                  class="detailPrompt running"
                  :title="`${formatLocalDateTime(selectedRunActivity.time)} · ${selectedRunActivity.summary}`"
                  >{{ selectedRunActivity.summary }}</span
                >
                <span
                  v-else-if="selectedRunInstruction"
                  class="detailPrompt"
                  :title="selectedRunInstruction"
                  >{{ selectedRunInstruction }}</span
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
                <details class="detailMore compact">
                  <summary title="More" aria-label="More">⋯</summary>
                  <div class="detailMorePopup">
                    <div class="detailPopupWorkdir mono" :title="selectedSession.workdir">
                      {{ selectedSession.workdir }}
                    </div>
                    <div class="detailMoreActions">
                      <button
                        type="button"
                        @click="setWorkspace(selectedSession.workdir)"
                        title="Focus workspace"
                      >
                        Focus workspace
                      </button>
                      <button
                        type="button"
                        @click="copyText(selectedSession.workdir)"
                        title="Copy workdir"
                      >
                        Copy workdir
                      </button>
                      <button
                        type="button"
                        @click="openSessionRename(selectedSession)"
                        title="Rename session"
                      >
                        Rename session
                      </button>
                      <button
                        type="button"
                        class="dangerBtn"
                        @click="openSessionDelete(selectedSession)"
                        title="Delete session (soft)"
                      >
                        Delete session
                      </button>
                    </div>
                    <template v-if="runWorkspace">
                      <div class="detailPopupWorkdir mono" :title="runWorkspace.run_workdir">
                        Run workspace: {{ runWorkspace.run_workdir }}
                      </div>
                      <div class="detailMoreActions">
                        <button
                          type="button"
                          @click="copyText(runWorkspace.run_workdir)"
                          title="Copy run workspace dir"
                        >
                          Copy run dir
                        </button>
                        <button
                          v-if="runWorkspace.status === 'active'"
                          type="button"
                          @click="onMergeRunWorkspace"
                          :disabled="runWorkspaceLoading"
                          title="Merge changes back"
                        >
                          Merge back
                        </button>
                        <button
                          v-if="runWorkspace.status === 'active'"
                          type="button"
                          class="dangerBtn"
                          @click="onDiscardRunWorkspace"
                          :disabled="runWorkspaceLoading"
                          title="Discard and delete run workspace"
                        >
                          Discard
                        </button>
                        <span class="tinyHint"
                          >{{ runWorkspace.kind }} · {{ runWorkspace.status }}</span
                        >
                      </div>
                      <div v-if="runWorkspaceConflict" class="tinyHint warn">
                        {{ conflictSummary(runWorkspaceConflict) }}
                      </div>
                      <div v-else-if="runWorkspaceError" class="tinyHint warn">
                        {{ runWorkspaceError }}
                      </div>
                    </template>
                    <div class="detailMoreGrid">
                      <div>
                        <span class="k">Session</span>
                        <span class="mono">{{
                          selectedSession.session_id || "(pending)"
                        }}</span>
                      </div>
                      <div>
                        <span class="k">Score</span> {{ selectedSession.score }} (stderr
                        {{ selectedSession.stderr_count }})
                      </div>
                      <div>
                        <span class="k">Status</span> {{ selectedSession.status }}
                      </div>
                      <div>
                        <span class="k">Runs</span> {{ selectedSession.runs.length }}
                      </div>
                      <div v-if="selectedSession.title" class="full">
                        <span class="k">Title</span>
                        <span>{{ selectedSession.title }}</span>
                      </div>
                      <div v-if="selectedSession.deleted_at" class="full">
                        <span class="k">Deleted</span>
                        <span class="mono">{{
                          formatLocalDateTime(selectedSession.deleted_at)
                        }}</span>
                      </div>
                      <div
                        v-if="selectedTask?.warning || selectedSession.warning"
                        class="full"
                      >
                        <span class="k">Warning</span>
                        {{ selectedTask?.warning || selectedSession.warning }}
                      </div>
                      <div v-if="selectedTask?.error" class="full">
                        <span class="k">Last Err</span> {{ selectedTask.error }}
                      </div>
                      <div class="full">
                        <span class="k">Prompt</span>
                        <span>{{ selectedSession.latest.prompt }}</span>
                      </div>
                    </div>
                  </div>
                </details>
              </div>
            </div>
          </div>

	          <div v-if="selectedTask?.status === 'blocked'" class="blockedHint">
	            <div class="text">
	              Blocked: requires approval · Try enabling <span class="mono">Install unlock</span> (one-time) or set
	              <span class="mono">workers.unsafe_automation</span> and retry (dangerous).
	            </div>
	            <div class="actions">
	              <button
	                type="button"
                @click="copyText('workers:\\n  unsafe_automation: true\\n')"
                title="Copy config snippet"
              >
                Copy snippet
              </button>
            </div>
          </div>

          <div v-if="acceptanceState || acceptanceLoading || acceptanceError" class="acceptanceHint">
            <div class="text">
              <span class="k">Acceptance</span>
              <span v-if="acceptanceLoading">Loading…</span>
              <template v-else-if="acceptanceError">
                {{ acceptanceError }}
              </template>
              <template v-else-if="acceptanceState">
                {{ acceptanceState.status }} · {{ acceptanceState.iteration }}/{{
                  acceptanceState.max_iterations
                }}
                <span v-if="acceptanceState.current_gate" class="mono"
                  >· {{ acceptanceState.current_gate }}</span
                >
                <span v-if="acceptanceState.summary">· {{ acceptanceState.summary }}</span>
              </template>
            </div>
            <div class="actions">
              <button type="button" @click="refreshAcceptance" :disabled="acceptanceLoading">
                Refresh
              </button>
              <button
                v-if="acceptanceState && (acceptanceState.report || acceptanceState.plan_json)"
                type="button"
                @click="acceptanceExpanded = !acceptanceExpanded"
              >
                {{ acceptanceExpanded ? "Hide" : "View" }}
              </button>
            </div>
          </div>

          <div
            v-if="acceptanceExpanded && acceptanceState && (acceptanceState.report || acceptanceState.plan_json)"
            class="acceptanceReport"
          >
            <div class="resultBox markdown" v-html="renderMarkdownSafe(acceptanceState.report || acceptanceState.plan_json || '')"></div>
          </div>

          <div class="resumeBar">
            <div class="resumeRow">
              <input
                v-if="!resumeExpanded"
                v-model="resumePrompt"
                placeholder="Continue with..."
                @keydown.enter="onResumeEnter"
              />
              <textarea
                v-else
                v-model="resumePrompt"
                rows="3"
                placeholder="Continue with..."
              ></textarea>
	              <div
	                v-if="resumeDriver === 'codex' || resumeDriver === 'claude-code'"
	                class="resumeSafetyControls"
	              >
	                <span class="pill" :class="resumeUseAutopilot ? 'low' : 'warn'">{{
	                  resumeUseAutopilot ? "Auto" : "Manual"
	                }}</span>
	                <span class="mono">
	                  <template v-if="resumeShowManualSafety">
	                    {{ resumeTaskIntent }}/{{ resumeSafetyPreset }}
	                  </template>
	                  <template v-else-if="selectedSession">
	                    {{
	                      normalizeTaskIntent(selectedSession.latest.task_intent ?? "code")
	                    }}/{{ effectiveSafetyPresetForTask(resumeDriver, selectedSession.latest) }}
	                  </template>
	                </span>
	              </div>
              <button
                type="button"
                class="primary"
                @click="onResumeTask"
                :disabled="!resumePrompt.trim() || !selectedSession.session_id || !!selectedSession.deleted_at || highRiskConfirmOpen"
	              >
                Resume
              </button>
              <button
                type="button"
                class="resumeToggle"
                @click="resumeExpanded = !resumeExpanded"
                :title="resumeExpanded ? 'Collapse' : 'Expand'"
              >
                {{ resumeExpanded ? "▴" : "⋯" }}
              </button>
            </div>
	            <div
	              v-if="resumeExpanded && (resumeDriver === 'codex' || resumeDriver === 'claude-code')"
	              class="resumeSafetyExtra"
	            >
	              <div class="resumeSafetyExtraGrid">
	                <label class="full">
	                  <input type="checkbox" v-model="runSafetyInstallUnlock" />
	                  <span class="mono">Install unlock</span>
	                  <span class="tinyHint">Allows downloads/installers (higher risk)</span>
	                </label>
	                <label class="full">
	                  <input type="checkbox" v-model="runSafetyAutopilotEnabled" />
	                  <span>Safety autopilot (recommended)</span>
	                </label>
	                <label v-if="runSafetyAutopilotEnabled" class="full">
	                  <input type="checkbox" v-model="resumeSafetyOverride" />
	                  <span>Override autopilot (manual intent/preset)</span>
	                </label>
	              </div>

	              <template v-if="resumeShowManualSafety">
	                <div class="resumeSafetyGrid">
	                  <label class="resumeSafetyLabel">
	                    Intent
	                    <select v-model="resumeTaskIntent">
	                      <option value="code">code</option>
	                      <option value="analyze">analyze</option>
	                      <option value="search-browse">search-browse</option>
	                      <option value="install">install</option>
	                    </select>
	                  </label>
	                  <label class="resumeSafetyLabel">
	                    Safety
	                    <select v-model="resumeSafetyPreset">
	                      <option
	                        v-for="p in safetyPresetsForDriver(resumeDriver)"
	                        :key="p.value"
	                        :value="p.value"
	                      >
	                        {{ p.value }}
	                      </option>
	                    </select>
	                  </label>
	                </div>
	                <div class="tinyHint">
	                  Recommended:
	                  <span class="mono">{{ recommendSafetyPreset(resumeDriver, resumeTaskIntent) }}</span>
	                  <button
	                    type="button"
	                    class="inlineBtn"
	                    @click="resumeSafetyPreset = recommendSafetyPreset(resumeDriver, resumeTaskIntent)"
	                  >
	                    Use
	                  </button>
	                </div>
	                <div
	                  v-if="resumeDriver === 'claude-code' && resumeSafetyPreset === 'search-browse'"
	                  class="tinyHint"
	                >
	                  Enables Claude Code WebFetch. Downloads via <span class="mono">curl</span>/<span class="mono">wget</span> remain denied by default.
	                </div>
	                <div
	                  v-else-if="resumeDriver === 'codex' && resumeSafetyPreset === 'search-browse'"
	                  class="tinyHint"
	                >
	                  Enables Codex <span class="mono">--search</span> (native web_search tool). Search/browse is distinct from downloading/executing scripts.
	                </div>
	                <div
	                  v-if="isHighRiskPreset(resumeDriver, resumeSafetyPreset)"
	                  class="resumeSafetyWarn"
	                >
	                  <div class="tinyHint warn">
	                    <template v-if="resumeDriver === 'codex' && resumeSafetyPreset === 'unsafe'">
	                      Runs Codex with <span class="mono">--dangerously-bypass-approvals-and-sandbox</span> (no sandbox).
	                    </template>
	                    <template v-else-if="resumeDriver === 'codex' && resumeSafetyPreset === 'danger-full-access'">
	                      Runs Codex with <span class="mono">--sandbox danger-full-access</span> (can access outside the workspace).
	                    </template>
	                    <template v-else-if="resumeDriver === 'claude-code' && resumeSafetyPreset === 'unsafe'">
	                      Runs Claude Code with <span class="mono">--dangerously-skip-permissions</span>. Recommended only for sandboxes with no internet access.
	                    </template>
	                  </div>
	                  <label class="resumeSafetyOptIn">
	                    <input type="checkbox" v-model="resumeHighRiskOptIn" />
	                    <span>I understand and want to proceed</span>
	                  </label>
	                </div>
	              </template>
	              <template v-else>
	                <div v-if="resumeAutopilotHighRiskBlocked" class="tinyHint warn">
	                  High-risk run detected. Enable <span class="mono">Install unlock</span> or use manual override.
	                </div>
	                <div class="tinyHint">
	                  Uses last run’s safety settings.
	                </div>
	              </template>
	            </div>
            <div v-if="selectedSession.deleted_at" class="tinyHint">
              session deleted：resume disabled
            </div>
            <div v-else-if="!selectedSession.session_id" class="tinyHint">
              session_id pending：暂时无法 resume
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
              <button
                type="button"
                class="tabBtn"
                :class="{ active: outputTab === 'trace' }"
                @click="outputTab = 'trace'"
              >
                Trace
              </button>
              <template v-if="outputTab === 'result'">
                <span class="tabDivider"></span>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: resultPreviewTab === 'markdown' }"
                  @click="resultPreviewTab = 'markdown'"
                  title="Markdown preview"
                >
                  Markdown
                </button>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: resultPreviewTab === 'raw' }"
                  @click="resultPreviewTab = 'raw'"
                  title="Raw text"
                >
                  Raw
                </button>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: resultPreviewTab === 'html' }"
                  @click="resultPreviewTab = 'html'"
                  title="HTML preview (sandboxed)"
                >
                  HTML
                </button>
              </template>
              <template v-else-if="outputTab === 'logs'">
                <span class="tabDivider"></span>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: logPreviewTab === 'pretty' }"
                  @click="logPreviewTab = 'pretty'"
                  title="Pretty view (summaries + expandable JSON)"
                >
                  Pretty
                </button>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: logPreviewTab === 'raw' }"
                  @click="logPreviewTab = 'raw'"
                  title="Raw text"
                >
                  Raw
                </button>
              </template>
              <div class="tabSpacer"></div>
              <button
                v-if="outputTab === 'result'"
                type="button"
                @click="copySelectedResult"
                :disabled="!selectedResultText"
              >
                Copy
              </button>
              <template v-else-if="outputTab === 'logs'">
                <button type="button" @click="copyFilteredLogs" :disabled="!filteredLogs.length">
                  Copy
                </button>
                <button type="button" @click="downloadSelectedLogs" :disabled="!selectedTask">
                  Download
                </button>
              </template>
              <button
                v-if="selectedTask"
                type="button"
                @click="replaySelectedRun"
                title="Replay run"
              >
                Replay
              </button>
            </div>

            <div v-if="outputTab === 'result'" class="resultPanel">
              <div
                v-if="runWorkspace && runWorkspace.status === 'active'"
                class="runWorkspaceBanner"
              >
                <div class="tinyHint">
                  Isolated run workspace:
                  <span class="mono" :title="runWorkspace.run_workdir">{{
                    workdirLabelForSession(runWorkspace.run_workdir)
                  }}</span>
                  · Base workdir:
                  <span class="mono" :title="selectedTask?.workdir">{{
                    workdirLabelForSession(selectedTask?.workdir ?? '')
                  }}</span>
                  · 工具把 run workspace 当作“当前目录”输出属正常现象
                </div>
                <div class="runWorkspaceBannerActions">
                  <button
                    type="button"
                    @click="setWorkspace(runWorkspace.run_workdir)"
                    title="Focus run workspace"
                  >
                    Focus run dir
                  </button>
                  <button
                    type="button"
                    @click="copyText(runWorkspace.run_workdir)"
                    title="Copy run workspace dir"
                  >
                    Copy run dir
                  </button>
                  <button
                    type="button"
                    @click="copyText(selectedTask?.workdir ?? '')"
                    :disabled="!selectedTask?.workdir"
                    title="Copy base workdir"
                  >
                    Copy base dir
                  </button>
                </div>
              </div>
              <div v-if="!selectedResultText" class="empty">
                {{
                  selectedTask?.status === "running" ||
                  selectedTask?.status === "queued"
                    ? "Task is running…"
                    : "No result yet."
                }}
              </div>
              <template v-else>
                <div
                  v-if="resultPreviewTab === 'markdown'"
                  class="resultBox markdown"
                  v-html="selectedResultHtml"
                  @click="onResultMarkdownClick"
                ></div>
                <div v-else-if="resultPreviewTab === 'raw'" class="resultBox">
                  <pre class="rawBox">{{ selectedResultText }}</pre>
                </div>
                <div v-else class="resultBox">
                  <iframe
                    class="htmlPreviewFrame"
                    sandbox
                    referrerpolicy="no-referrer"
                    :srcdoc="selectedResultHtmlSrcDoc"
                    title="HTML preview"
                  ></iframe>
                </div>
              </template>
            </div>

            <div v-else-if="outputTab === 'trace'" class="tracePanel">
              <div v-if="traceError" class="modalError">{{ traceError }}</div>
              <div v-else-if="traceLoading" class="loading">Loading...</div>
              <template v-else>
                <div v-if="!selectedTrace?.invocation" class="empty">
                  No trace yet.
                </div>
                <div v-else class="traceBox">
                  <div class="traceRow">
                    <span class="k">cmd</span>
                    <span class="mono">{{ selectedTrace.invocation.cmd }}</span>
                  </div>
                  <div class="traceRow">
                    <span class="k">dir</span>
                    <span class="mono">{{ selectedTrace.invocation.dir }}</span>
                  </div>
                  <div class="traceRow">
                    <span class="k">args</span>
                    <span class="mono">{{ (selectedTrace.invocation.args ?? []).join(" ") }}</span>
                  </div>
                  <div class="traceRow">
                    <span class="k">env</span>
                    <div class="traceEnv">
                      <span
                        v-for="k in selectedTrace.invocation.env_injected_keys ?? []"
                        :key="k"
                        class="pill mono"
                        >{{ k }}</span
                      >
                      <span v-if="!(selectedTrace.invocation.env_injected_keys ?? []).length" class="tinyHint"
                        >none</span
                      >
                    </div>
                  </div>
                </div>
              </template>
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

              <div class="logbox" :class="logPreviewTab">
                <template v-if="logPreviewTab === 'raw'">
                  <div
                    v-for="l in filteredLogs"
                    :key="l.id"
                    class="logLine"
                    :class="`s-${l.stream}`"
                  >
                    <span class="logTime" :title="formatLocalDateTime(l.time)">{{
                      formatLogTime(l.time)
                    }}</span>
                    <span class="logTag" :class="l.stream">{{ l.stream }}</span>
                    <span class="logMsg">{{ l.message }}</span>
                  </div>
                </template>
                <template v-else>
                  <details
                    v-for="p in prettyLogs"
                    :key="p.id"
                    class="logEvent"
                    :class="`s-${p.stream}`"
                  >
                    <summary class="logEventSummary">
                      <span class="logTime" :title="formatLocalDateTime(p.time)">{{
                        formatLogTime(p.time)
                      }}</span>
                      <span class="logTag" :class="p.stream">{{ p.stream }}</span>
                      <span class="logSummary">{{ p.summary }}</span>
                    </summary>
                    <div class="logEventBody">
                      <pre v-if="p.kind === 'text'" class="logDetail">{{ p.details }}</pre>
                      <pre v-else class="hljs logDetail"><code v-html="p.jsonHtml"></code></pre>
                    </div>
                  </details>
                </template>
              </div>
            </div>
          </div>
        </div>
      </section>
      </template>
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

    <div v-if="feedCoachOpen" class="feedCoach" role="note">
      <div class="feedCoachText">
        Live Feed available · press <span class="mono">L</span> or click
        <span class="mono">Live</span>
      </div>
      <div class="feedCoachActions">
        <button type="button" class="primary" @click="openLive">Open</button>
        <button type="button" @click="dismissFeedCoach">✕</button>
      </div>
    </div>

    <div v-if="deliveryForemanToastOpen" class="foremanToast" role="status">
      <div class="foremanText">{{ deliveryForemanToast }}</div>
      <div class="foremanActions">
        <button type="button" class="primary" @click="openSecretaryForForeman">
          Open
        </button>
        <button type="button" @click="deliveryForemanToastOpen = false">✕</button>
      </div>
    </div>

	    <SecretaryDrawer
	      v-if="secretaryOpen"
	      v-model:full="secretaryFull"
	      v-model:view="secretaryView"
	      v-model:scope="secretaryScope"
	      v-model:autopilotEnabled="attentionAutopilotEnabled"
	      v-model:chatBackend="chatBackend"
	      v-model:chatStreamEnabled="chatStreamEnabled"
	      v-model:chatMaxSteps="chatMaxSteps"
	      v-model:chatInput="chatInput"
	      :width="secretaryWidth"
	      :resizing="secretaryResizing"
	      :counts="secretaryCounts"
	      :needsAttentionSessions="needsAttentionSessions"
	      :autopilotNote="attentionAutopilotNote"
	      :briefing="secretaryBriefing"
	      :chat="chat"
	      :chatStreamStatus="chatStreamStatus"
	      :chatStreamAnswer="chatStreamAnswer"
	      :chatSending="chatSending"
	      :theme="theme"
	      :renderMarkdownSafe="renderMarkdownSafe"
	      @close="closeSecretary"
	      @startResize="startSecretaryResize"
	      @selectTask="onSelectTask"
	      @resumeSession="secretaryResumeSessionRun"
	      @cancelSession="secretaryCancelSessionRun"
	      @sendChat="sendChatMessage"
	      @markdownClick="onResultMarkdownClick"
	    />

		    <LiveDrawer
		      v-if="liveOpen"
		      v-model:full="liveFull"
		      v-model:scope="liveScope"
		      v-model:mode="liveMode"
		      v-model:wrap="liveWrap"
		      v-model:paused="livePaused"
		      :width="liveWidth"
		      :resizing="liveResizing"
		      :items="liveItems"
		      :eventsConnected="eventsConnected"
		      :eventsIdleSeconds="eventsIdleSeconds"
		      :feedIdleSeconds="feedIdleSeconds"
		      :selectedTaskStatus="selectedTask?.status ?? ''"
		      :boxElRef="liveBoxEl"
		      :formatLogTime="formatLogTime"
		      :formatLocalDateTime="formatLocalDateTime"
		      @close="liveOpen = false"
		      @reconnect="reconnectEvents"
		      @startResize="startLiveResize"
		    />

		    <div v-if="runsOpen" class="modalOverlay" @click.self="runsOpen = false">
		      <div class="modal runsModal">
		        <div class="modalHeader">
		          <div class="modalTitle">
		            Runs <span class="runsCount">{{ selectedSession?.runs.length ?? 0 }}</span>
	          </div>
	          <button class="iconBtn" type="button" @click="runsOpen = false">✕</button>
	        </div>
	        <div class="modalBody runsModalBody">
	          <div v-if="!selectedSession" class="empty">No session selected</div>
	          <div v-else class="runList runsModalList">
	            <button
	              v-for="r in selectedSession.runs.slice().reverse()"
	              :key="r.id"
	              type="button"
	              class="runRow"
	              :class="{ active: r.id === selectedTaskId }"
	              @click="onSelectTask(r.id)"
	            >
	              <div class="runTop">
                  <div class="runTopLeft">
                    <span class="mono runId">{{ r.id.slice(0, 8) }}</span>
                    <span class="pill kind">{{ r.mode }}</span>
                    <span class="score">score {{ r.score }}</span>
                    <span class="mono runTime" :title="r.created_at">{{
                      formatLocalDateTime(r.created_at)
                    }}</span>
                  </div>
                  <span class="pill" :class="r.status">{{ r.status }}</span>
	              </div>
	              <div class="runBottom">{{ r.prompt }}</div>
	            </button>
	          </div>
	        </div>
	        <div class="modalFooter">
	          <button type="button" @click="runsOpen = false">Close</button>
	        </div>
		      </div>
		    </div>

        <div
          v-if="workspaceMergePromptOpen"
          class="modalOverlay"
          @click.self="closeWorkspaceMergePrompt"
        >
          <div class="modal smallModal">
            <div class="modalHeader">
              <div class="modalTitle">Merge run workspace</div>
              <button class="iconBtn" type="button" @click="closeWorkspaceMergePrompt">
                ✕
              </button>
            </div>
            <div class="modalBody">
              <div v-if="workspaceMergePromptError" class="modalError">
                {{ workspaceMergePromptError }}
              </div>
              <div class="confirmText">
                This run used an isolated workspace. Merge changes back into your base workdir/branch?
              </div>
              <div
                v-if="workspaceMergePromptWorkspace"
                class="tinyHint"
                style="overflow-wrap: anywhere"
              >
                base_workdir:
                <span class="mono">{{ workspaceMergePromptWorkspace.base_workdir }}</span>
              </div>
              <div
                v-if="workspaceMergePromptWorkspace"
                class="tinyHint"
                style="overflow-wrap: anywhere"
              >
                run_workdir:
                <span class="mono">{{ workspaceMergePromptWorkspace.run_workdir }}</span>
              </div>
              <div v-if="runWorkspaceConflict" class="modalError">
                {{ conflictSummary(runWorkspaceConflict) }}
              </div>
              <div class="tinyHint">
                Tip: you can also merge later from the session “⋯” menu.
              </div>
            </div>
            <div class="modalFooter">
              <button type="button" @click="closeWorkspaceMergePrompt" :disabled="workspaceMergePromptBusy">
                Not now
              </button>
              <button
                type="button"
                class="primary"
                @click="confirmWorkspaceMergePrompt"
                :disabled="workspaceMergePromptBusy"
              >
                {{ workspaceMergePromptBusy ? "Merging..." : "Merge back" }}
              </button>
            </div>
          </div>
        </div>

        <div
          v-if="rehydratePromptOpen"
          class="modalOverlay"
          @click.self="closeRehydratePrompt"
        >
          <div class="modal smallModal">
            <div class="modalHeader">
              <div class="modalTitle">无法恢复该会话</div>
              <button class="iconBtn" type="button" @click="closeRehydratePrompt">✕</button>
            </div>
            <div class="modalBody">
              <div v-if="rehydratePromptError" class="modalError">
                {{ rehydratePromptError }}
              </div>
              <div class="confirmText">
                Claude Code 找不到该 session（No conversation found）。你可以新建一个会话，把历史上下文带过去继续。
              </div>
              <div v-if="rehydratePromptWorkspace?.status === 'active'" class="modalError">
                检测到该会话仍有隔离工作区处于 <span class="mono">active</span>。为避免丢改动，请先在
                Workspace 面板点击「Merge」把改动合并回 <span class="mono">base_workdir</span>，然后再继续。
              </div>
              <div
                v-if="rehydratePromptWorkspace"
                class="tinyHint"
                style="overflow-wrap: anywhere"
              >
                base_workdir:
                <span class="mono">{{ rehydratePromptWorkspace.base_workdir }}</span>
              </div>
              <div class="tinyHint">
                说明：该操作会创建一个新的 <span class="mono">mode=new</span> run（不会复用旧
                <span class="mono">session_id</span>）。
              </div>
            </div>
            <div class="modalFooter">
              <button type="button" @click="closeRehydratePrompt" :disabled="rehydratePromptBusy">
                取消
              </button>
              <button
                type="button"
                class="primary"
                @click="confirmRehydratePrompt"
                :disabled="rehydratePromptBusy || rehydratePromptWorkspace?.status === 'active'"
              >
                {{ rehydratePromptBusy ? "创建中..." : "新建会话继续（带上下文）" }}
              </button>
            </div>
          </div>
        </div>

	      <div
	        v-if="workspaceRenameOpen"
	        class="modalOverlay"
	        @click.self="closeWorkspaceRename"
	      >
        <div class="modal smallModal">
          <div class="modalHeader">
            <div class="modalTitle">Rename Workspace</div>
            <button class="iconBtn" type="button" @click="closeWorkspaceRename">
              ✕
            </button>
          </div>
          <div class="modalBody">
            <div class="tinyHint">
              Workspace: <span class="mono">{{ workspaceRenamePath }}</span>
            </div>
            <label class="full">
              Name
              <input
                ref="workspaceRenameInputEl"
                v-model="workspaceRenameValue"
                placeholder="(optional)"
              />
            </label>
            <div class="tinyHint">
              Name is stored locally and shown in the workspace selector / pinned list.
            </div>
          </div>
          <div class="modalFooter">
            <button type="button" @click="closeWorkspaceRename">Cancel</button>
            <button type="button" class="primary" @click="saveWorkspaceRename">
              Save
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="sessionRenameOpen"
        class="modalOverlay"
        @click.self="closeSessionRename"
      >
        <div class="modal smallModal">
          <div class="modalHeader">
            <div class="modalTitle">Rename Session</div>
            <button class="iconBtn" type="button" @click="closeSessionRename">
              ✕
            </button>
          </div>
          <div class="modalBody">
            <div v-if="sessionRenameError" class="modalError">
              {{ sessionRenameError }}
            </div>
            <label class="full">
              Title
              <input
                ref="sessionRenameInputEl"
                v-model="sessionRenameTitle"
                placeholder="(optional)"
              />
            </label>
            <div class="tinyHint">
              Title is stored locally (soft) and shown in Sessions list/detail.
            </div>
          </div>
          <div class="modalFooter">
            <button type="button" @click="closeSessionRename">Cancel</button>
            <button
              type="button"
              class="primary"
              @click="saveSessionRename"
              :disabled="sessionRenameSaving"
            >
              Save
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="sessionDeleteOpen"
        class="modalOverlay"
        @click.self="closeSessionDelete"
      >
        <div class="modal smallModal">
          <div class="modalHeader">
            <div class="modalTitle">Delete Session</div>
            <button class="iconBtn" type="button" @click="closeSessionDelete">
              ✕
            </button>
          </div>
          <div class="modalBody">
            <div v-if="sessionDeleteError" class="modalError">
              {{ sessionDeleteError }}
            </div>
            <div class="confirmText">
              Delete <span class="mono">{{ sessionDeleteLabel }}</span> ? (soft delete)
            </div>
            <div class="tinyHint">
              Deleted sessions are hidden by default. You can enable “Show deleted” to view them.
            </div>
          </div>
          <div class="modalFooter">
            <button type="button" @click="closeSessionDelete">Cancel</button>
            <button
              type="button"
              class="dangerBtn"
              @click="confirmSessionDelete"
              :disabled="sessionDeleteSaving"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
	
	    <div
	      v-if="newRunOpen"
	      class="modalOverlay newRunOverlay"
      @click.self="closeNewRun"
    >
      <div class="modal newRunModal">
        <div class="modalHeader">
          <div class="modalTitle">New Run</div>
          <button class="iconBtn" type="button" @click="closeNewRun">✕</button>
        </div>

	        <div class="modalBody newRunBody">
	          <div class="form newRunForm">
	            <label class="full">
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
	            <details class="newRunAdvanced full">
	              <summary>
	                <span>Advanced</span>
	                <span class="newRunAdvancedSummaryHint">
	                  <template v-if="toolsError">Tools error</template>
	                  <template v-else-if="toolsLoading">Loading tools…</template>
	                  <template v-else>Tool: <span class="mono">{{ newWorkerType }}</span></template>
	                </span>
	              </summary>
	              <div class="newRunAdvancedBody">
	                <label>
	                  Tool
	                  <select v-model="newWorkerType">
	                    <option v-for="t in toolsList" :key="t.id" :value="t.id">
	                      {{ t.id }}
	                    </option>
	                  </select>
	                </label>
	                <div v-if="toolsError" class="tinyHint warn">{{ toolsError }}</div>
	                <div v-else-if="toolsLoading" class="tinyHint">Loading tools…</div>
	                <div v-else-if="newTool" class="tinyHint">
	                  driver: <span class="mono">{{ newTool.driver }}</span> · cmd:
	                  <span class="mono">{{ newTool.command }}</span>
	                </div>

	                <div
	                  v-if="newRunDriver === 'codex' || newRunDriver === 'claude-code'"
	                  class="newRunSafety"
	                >
	                  <div class="newRunSafetyTitle">Safety</div>
	                  <div class="newRunSafetyAutoRow">
	                    <span
	                      class="pill"
	                      :class="runSafetyAutopilotEnabled ? 'low' : 'warn'"
	                      >Autopilot</span
	                    >
	                    <span class="tinyHint">
	                      <template v-if="runSafetyAutopilotEnabled">
	                        Infers intent from the prompt and applies best-practice sandbox defaults.
	                      </template>
	                      <template v-else>
	                        Autopilot disabled: choose intent/preset below.
	                      </template>
	                    </span>
	                  </div>

	                  <label class="newRunSafetyUnlock">
	                    <input type="checkbox" v-model="runSafetyInstallUnlock" />
	                    <span class="mono">Install unlock</span>
	                    <span class="tinyHint">Allows downloads/installers (higher risk)</span>
	                  </label>

	                  <div class="newRunSafetyAdvanced">
	                    <div class="newRunSafetyAdvancedGrid">
	                      <label class="full">
	                        <input type="checkbox" v-model="runSafetyAutopilotEnabled" />
	                        <span>Safety autopilot (recommended)</span>
	                      </label>
	                      <label v-if="runSafetyAutopilotEnabled" class="full">
	                        <input type="checkbox" v-model="newRunSafetyOverride" />
	                        <span>Override autopilot (manual intent/preset)</span>
	                      </label>
	                    </div>

	                    <template v-if="newRunShowManualSafety">
	                      <div class="newRunSafetyGrid">
	                        <label>
	                          Task intent
	                          <select v-model="newRunTaskIntent">
	                            <option value="code">code</option>
	                            <option value="analyze">analyze</option>
	                            <option value="search-browse">search-browse</option>
	                            <option value="install">install</option>
	                          </select>
	                        </label>
	                        <label>
	                          Safety preset
	                          <select v-model="newRunSafetyPreset">
	                            <option
	                              v-for="p in safetyPresetsForDriver(newRunDriver)"
	                              :key="p.value"
	                              :value="p.value"
	                            >
	                              {{ p.value }}
	                            </option>
	                          </select>
	                        </label>
	                      </div>
	                      <div class="tinyHint">
	                        Recommended:
	                        <span class="mono">{{ recommendSafetyPreset(newRunDriver, newRunTaskIntent) }}</span>
	                        <button
	                          type="button"
	                          class="inlineBtn"
	                          @click="newRunSafetyPreset = recommendSafetyPreset(newRunDriver, newRunTaskIntent)"
	                        >
	                          Use
	                        </button>
	                      </div>

	                      <div
	                        v-if="newRunDriver === 'claude-code' && newRunSafetyPreset === 'search-browse'"
	                        class="tinyHint"
	                      >
	                        Enables Claude Code WebFetch. Downloads via <span class="mono">curl</span>/<span class="mono">wget</span> remain denied by default.
	                      </div>
	                      <div
	                        v-else-if="newRunDriver === 'codex' && newRunSafetyPreset === 'search-browse'"
	                        class="tinyHint"
	                      >
	                        Enables Codex <span class="mono">--search</span> (native web_search tool). Search/browse is distinct from downloading/executing scripts.
	                      </div>

	                      <div
	                        v-if="isHighRiskPreset(newRunDriver, newRunSafetyPreset)"
	                        class="newRunSafetyWarn"
	                      >
	                        <div class="tinyHint warn">
	                          <template v-if="newRunDriver === 'codex' && newRunSafetyPreset === 'unsafe'">
	                            Runs Codex with <span class="mono">--dangerously-bypass-approvals-and-sandbox</span> (no sandbox).
	                          </template>
	                          <template v-else-if="newRunDriver === 'codex' && newRunSafetyPreset === 'danger-full-access'">
	                            Runs Codex with <span class="mono">--sandbox danger-full-access</span> (can access outside the workspace).
	                          </template>
	                          <template v-else-if="newRunDriver === 'claude-code' && newRunSafetyPreset === 'unsafe'">
	                            Runs Claude Code with <span class="mono">--dangerously-skip-permissions</span>. Recommended only for sandboxes with no internet access.
	                          </template>
	                        </div>
	                        <label class="newRunSafetyOptIn">
	                          <input type="checkbox" v-model="newRunHighRiskOptIn" />
	                          <span>I understand and want to proceed</span>
	                        </label>
	                      </div>
	                    </template>
	                  </div>
	                </div>

	                <div class="newRunHint">
	                  Hotkeys: <span class="mono">N</span> new run ·
	                  <span class="mono">S</span> secretary ·
	                  <span class="mono">L</span> live ·
	                  <span class="mono">Esc</span> close
	                </div>
	              </div>
	            </details>
	          </div>
	        </div>

        <div class="modalFooter">
          <button type="button" @click="closeNewRun">Cancel</button>
          <button
            type="button"
            class="primary"
            @click="onCreateTaskFromModal"
            :disabled="!newPrompt.trim() || !newWorkdir.trim() || !!missingAuthText || highRiskConfirmOpen"
	          >
	            Start
	          </button>
        </div>
      </div>
    </div>

    <div
      v-if="highRiskConfirmOpen"
      class="modalOverlay"
      @click.self="cancelHighRiskConfirm"
    >
      <div class="modal smallModal">
        <div class="modalHeader">
          <div class="modalTitle">{{ highRiskConfirmTitle }}</div>
          <button class="iconBtn" type="button" @click="cancelHighRiskConfirm">
            ✕
          </button>
        </div>
        <div class="modalBody">
          <div class="confirmText">{{ highRiskConfirmMessage }}</div>
          <div v-if="highRiskConfirmDetail" class="tinyHint warn mono">
            {{ highRiskConfirmDetail }}
          </div>
        </div>
        <div class="modalFooter">
          <button type="button" @click="cancelHighRiskConfirm">Cancel</button>
          <button
            type="button"
            class="dangerBtn"
            @click="confirmHighRiskConfirm"
            :disabled="highRiskConfirmBusy"
          >
            {{ highRiskConfirmConfirmLabel }}
          </button>
        </div>
      </div>
    </div>

	    <AuthSettingsModal
	      :open="authSettingsOpen"
	      :saving="authSaving"
	      :error="authSettingsError"
	      :storagePath="authInfo?.storage_path ?? ''"
	      :authStatus="authStatus"
	      v-model:autoDeliveryForeman="autoDeliveryForeman"
	      v-model:anthropicBaseURL="authAnthropicBaseURL"
	      v-model:anthropicApiKey="authAnthropicApiKey"
	      v-model:anthropicAuthToken="authAnthropicAuthToken"
	      v-model:anthropicModel="authAnthropicModel"
	      v-model:anthropicSmallFastModel="authAnthropicSmallFastModel"
	      v-model:openAIApiKey="authOpenAIApiKey"
	      v-model:codexModel="authCodexModel"
	      v-model:codexReasoningEffort="authCodexReasoningEffort"
	      @close="authSettingsOpen = false"
	      @openTools="openToolsSettings"
	      @save="saveAuthSettings"
	      @clearStored="clearStoredAuth"
	    />

	    <ToolsSettingsModal
	      :open="toolsSettingsOpen"
	      :loading="toolsLoading"
	      :saving="toolsSaving"
	      :error="toolsError"
	      :tools="toolsList"
	      v-model:editID="toolEditID"
	      v-model:editDriver="toolEditDriver"
	      v-model:editCommand="toolEditCommand"
	      v-model:editArgs="toolEditArgs"
	      v-model:editEnv="toolEditEnv"
	      @close="toolsSettingsOpen = false"
	      @newTool="startNewTool"
	      @refresh="refreshTools"
	      @selectTool="loadToolIntoEditor"
	      @delete="deleteToolOverride"
	      @save="saveTool"
	    />

	    <div
	      v-if="filePreviewOpen"
	      class="modalOverlay"
	      @click.self="closeFilePreview"
	    >
      <div class="modal fileModal">
        <div class="modalHeader">
          <div class="modalTitle mono">
            {{ filePreviewResolvedPath || filePreviewRawPath }}
          </div>
          <button class="iconBtn" type="button" @click="closeFilePreview">
            ✕
          </button>
        </div>

        <div class="modalBody fileModalBody">
          <div v-if="filePreviewError" class="modalError">
            {{ filePreviewError }}
          </div>
          <div v-else-if="filePreviewLoading" class="loading">Loading...</div>
          <template v-else>
            <div class="fileMetaRow">
              <div class="fileMetaLeft">
                <span class="tinyHint mono">{{ filePreviewSize }} bytes</span>
                <span v-if="filePreviewTruncated" class="pill warn"
                  >truncated</span
                >
              </div>
              <div class="fileMetaActions">
                <button
                  type="button"
                  @click="copyText(filePreviewContent)"
                  :disabled="!filePreviewContent"
                >
                  Copy
                </button>
              </div>
            </div>

            <div class="outputTabs">
              <button
                type="button"
                class="tabBtn"
                :class="{ active: filePreviewTab === 'preview' }"
                @click="filePreviewTab = 'preview'"
              >
                Preview
              </button>
              <button
                type="button"
                class="tabBtn"
                :class="{ active: filePreviewTab === 'raw' }"
                @click="filePreviewTab = 'raw'"
              >
                Raw
              </button>
              <button
                type="button"
                class="tabBtn"
                :class="{ active: filePreviewTab === 'html' }"
                @click="filePreviewTab = 'html'"
                title="HTML preview (sandboxed)"
              >
                HTML
              </button>
              <div class="tabSpacer"></div>
            </div>

            <template v-if="filePreviewTab === 'raw'">
              <div class="resultBox">
                <pre class="rawBox">{{ filePreviewContent }}</pre>
              </div>
            </template>
            <template v-else-if="filePreviewTab === 'html'">
              <div class="resultBox">
                <iframe
                  class="htmlPreviewFrame"
                  sandbox
                  referrerpolicy="no-referrer"
                  :srcdoc="filePreviewHtmlSrcDoc"
                  title="HTML preview"
                ></iframe>
              </div>
            </template>
            <template v-else>
              <div
                v-if="filePreviewIsMarkdown"
                ref="filePreviewBoxEl"
                class="resultBox markdown filePreviewBox"
                v-html="filePreviewMarkdownHtml"
                @click="onFilePreviewMarkdownClick"
              ></div>

              <div v-else class="resultBox fileCodeBox">
                <pre class="hljs"><code v-html="filePreviewCodeHtml"></code></pre>
              </div>
            </template>
          </template>
        </div>

        <div class="modalFooter">
          <button type="button" @click="closeFilePreview">Close</button>
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

    <teleport to="body">
      <div
        v-if="sessionActionsMenuOpen"
        class="menuOverlay"
        @mousedown="closeSessionActionsMenu"
      ></div>
      <div
        v-if="sessionActionsMenuOpen && sessionActionsMenuSession"
        ref="sessionActionsMenuEl"
        class="rowMorePopup menuPopup"
        :style="{
          left: sessionActionsMenuPos.left + 'px',
          top: sessionActionsMenuPos.top + 'px',
        }"
        @mousedown.stop
      >
        <button
          type="button"
          @click="
            openSessionRename(sessionActionsMenuSession);
            closeSessionActionsMenu();
          "
        >
          Rename
        </button>
        <button
          type="button"
          class="dangerBtn"
          @click="
            openSessionDelete(sessionActionsMenuSession);
            closeSessionActionsMenu();
          "
        >
          Delete
        </button>
      </div>
    </teleport>
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
  min-height: 100dvh;
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

.headerLeft {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
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

.titleBtn {
  border: none;
  background: transparent;
  padding: 0;
  cursor: pointer;
  font: inherit;
  font-weight: 800;
  font-size: 20px;
  background: linear-gradient(135deg, #0d9488 0%, #0ea5e9 100%);
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
}

.titleBtn:hover {
  opacity: 0.92;
}

.titleBtn:focus-visible {
  outline: 2px solid rgba(14, 165, 233, 0.55);
  outline-offset: 3px;
  border-radius: 10px;
}

.homeStart {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.homeHero {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.homeTitle {
  font-size: 18px;
  font-weight: 800;
  color: var(--text-main);
}

.homeSub {
  color: var(--text-sub);
  font-size: 13px;
  font-weight: 500;
}

.homeStatus {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  background: var(--bg-subtle);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
}

.homeStatusText {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.homeStatusActions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}

.homeActions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
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

.menuBtn {
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  color: var(--text-main);
  border-radius: 12px;
  width: 40px;
  height: 40px;
  padding: 0;
  display: grid;
  place-items: center;
}

.menuBtn:hover {
  background: var(--bg-subtle);
  color: var(--color-primary);
}

.menuIcon {
  font-weight: 900;
  font-size: 18px;
  line-height: 1;
}

.headerMore {
  position: relative;
}

.headerMoreBtn {
  list-style: none;
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  color: var(--text-main);
  border-radius: 12px;
  width: 40px;
  height: 40px;
  padding: 0;
  display: grid;
  place-items: center;
  cursor: pointer;
}

.headerMoreBtn::-webkit-details-marker {
  display: none;
}

.headerMoreBtn:hover {
  background: var(--bg-subtle);
  color: var(--color-primary);
}

.headerMorePopup {
  position: absolute;
  right: 0;
  top: calc(100% + 10px);
  min-width: 180px;
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 8px;
  box-shadow: var(--shadow-lg);
  z-index: 160;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.headerMoreItem {
  border: 1px solid rgba(148, 163, 184, 0.22);
  background: var(--bg-subtle);
  color: var(--text-main);
  font-weight: 700;
  font-size: 13px;
  text-align: left;
  border-radius: 12px;
  padding: 10px 12px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.headerMoreItem:hover {
  border-color: rgba(45, 212, 191, 0.35);
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

.liveBtn {
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  color: var(--text-sub);
  font-weight: 800;
  font-size: 12px;
  border-radius: 999px;
  padding: 6px 12px;
  cursor: pointer;
  transition: all 0.2s;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.liveBtn:hover {
  color: var(--color-primary);
  border-color: rgba(45, 212, 191, 0.35);
  background: var(--bg-subtle);
}

.liveDot {
  font-size: 10px;
  line-height: 1;
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

.grid.gridSingle {
  grid-template-columns: 1fr;
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
  container-type: inline-size;
}

.skillsPagePanel {
  grid-column: 1 / -1;
  max-height: calc(100vh - 110px);
  max-height: calc(100dvh - 110px);
}

.filesPagePanel {
  grid-column: 1 / -1;
  max-height: calc(100vh - 110px);
  max-height: calc(100dvh - 110px);
}

.filesPageBody {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* Sticky sidebars */
.grid > .sessionsPanel {
  position: sticky;
  top: 90px; /* Header height + spacing */
  max-height: calc(100vh - 110px);
  max-height: calc(100dvh - 110px);
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

.h2Spacer {
  flex: 1;
}

.h2Meta {
  font-size: 12px;
  font-weight: 800;
  color: rgba(255, 255, 255, 0.85);
  background: rgba(15, 23, 42, 0.22);
  border: 1px solid rgba(255, 255, 255, 0.18);
  padding: 4px 8px;
  border-radius: 999px;
}

.h2Btn {
  border: 1px solid rgba(255, 255, 255, 0.25);
  background: rgba(255, 255, 255, 0.12);
  color: rgba(255, 255, 255, 0.9);
  font-weight: 800;
  font-size: 12px;
  border-radius: 999px;
  padding: 6px 10px;
}

.h2Btn:hover {
  border-color: rgba(255, 255, 255, 0.35);
  background: rgba(255, 255, 255, 0.16);
  color: white;
}

.sessionsOverlay {
  position: fixed;
  inset: 0;
  background: rgba(2, 6, 23, 0.55);
  backdrop-filter: blur(2px);
  z-index: 180;
}

.sessionsPanel.sessionsDrawerPanel {
  position: fixed;
  top: calc(76px + env(safe-area-inset-top));
  left: 12px;
  right: 12px;
  bottom: max(12px, env(safe-area-inset-bottom));
  z-index: 190;
  max-height: none;
  overflow: hidden;
  box-shadow: var(--shadow-lg);
}

.sessionsPanel.sessionsDrawerPanel .list {
  overflow: auto;
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

:deep(input:not([type="checkbox"]):not([type="radio"])),
:deep(select),
:deep(textarea) {
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

:deep(input:not([type="checkbox"]):not([type="radio"]):focus),
:deep(select:focus),
:deep(textarea:focus) {
  border-color: var(--color-primary);
  background: var(--bg-panel);
  box-shadow: 0 0 0 3px var(--color-primary-bg);
}

:deep(input[type="checkbox"]),
:deep(input[type="radio"]) {
  accent-color: var(--color-primary);
}

:deep(textarea) {
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

:deep(textarea:hover) {
  border-color: #94a3b8;
  background-color: var(--bg-panel);
}

:deep(textarea:focus) {
  border-color: var(--color-primary);
  background-color: var(--bg-panel);
  box-shadow: 0 0 0 3px var(--color-primary-bg), inset 0 1px 2px rgba(0, 0, 0, 0.04);
}

:deep(textarea::placeholder) {
  color: #94a3b8;
  font-style: italic;
}

:deep(button) {
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

:deep(button:hover:not(:disabled)) {
  background: var(--bg-subtle);
  border-color: rgba(148, 163, 184, 0.5);
  color: var(--color-primary);
}

:deep(button:disabled) {
  opacity: 0.6;
  cursor: not-allowed;
  background: var(--bg-subtle);
}

:deep(button.primary) {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
  box-shadow: 0 2px 4px rgba(13, 148, 136, 0.2);
}

:deep(button.primary:disabled) {
  background: #94a3b8;
  border-color: #94a3b8;
  opacity: 1;
  box-shadow: none;
}

:deep(button.primary:hover:not(:disabled)) {
  background: var(--color-primary-hover);
  border-color: var(--color-primary-hover);
  transform: translateY(-1px);
  box-shadow: 0 4px 6px rgba(13, 148, 136, 0.3);
  color: white;
}

:deep(button.primary:active:not(:disabled)) {
  transform: translateY(0);
}

:deep(button.dangerBtn) {
  background: #ef4444;
  color: white;
  border-color: #ef4444;
  box-shadow: 0 2px 4px rgba(239, 68, 68, 0.2);
}

:deep(button.dangerBtn:hover:not(:disabled)) {
  background: #dc2626;
  border-color: #dc2626;
  box-shadow: 0 4px 6px rgba(220, 38, 38, 0.3);
  color: white;
  transform: translateY(-1px);
}

:deep(button.dangerBtn:active:not(:disabled)) {
  transform: translateY(0);
}

:deep(button.dangerBtn:disabled) {
  background: #94a3b8;
  border-color: #94a3b8;
  opacity: 1;
  box-shadow: none;
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

.filtersBlock {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-subtle);
  padding: 12px;
  display: grid;
  gap: 12px;
}

.filtersActions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.filtersTitle {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 6px;
}

.loadMore {
  margin-top: 6px;
  width: 100%;
  padding: 10px 12px;
  font-weight: 800;
}

.newRunBody {
  padding: 0;
  overflow: auto;
}

.modal.newRunModal {
  height: min(680px, 92vh);
}

.newRunForm {
  padding: 20px;
  margin: 0;
  min-height: 0;
}

.newRunAdvanced {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-subtle);
  padding: 12px 14px;
  display: grid;
  gap: 10px;
}

.newRunAdvanced summary {
  cursor: pointer;
  font-size: 13px;
  font-weight: 800;
  user-select: none;
}

.newRunAdvancedSummaryHint {
  margin-left: 10px;
  font-size: 12px;
  font-weight: 700;
  color: var(--text-sub);
}

.newRunAdvancedBody {
  border-top: 1px dashed rgba(148, 163, 184, 0.35);
  padding-top: 12px;
  display: grid;
  gap: 12px;
}

.newRunSafety {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-subtle);
  padding: 12px 14px;
  display: grid;
  gap: 10px;
}

.newRunSafetyTitle {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.newRunSafetyGrid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-items: end;
}

.newRunSafetyAutoRow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.newRunSafetyUnlock {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 700;
  user-select: none;
  flex-wrap: wrap;
}

.newRunSafetyUnlock input {
  width: 18px;
  height: 18px;
}

.newRunSafetyAdvanced {
  border-top: 1px dashed rgba(148, 163, 184, 0.35);
  padding-top: 10px;
  display: grid;
  gap: 10px;
}

.newRunSafetyAdvanced summary {
  cursor: pointer;
  font-size: 13px;
  font-weight: 800;
  user-select: none;
}

.newRunSafetyAdvancedGrid {
  display: grid;
  gap: 8px;
}

.newRunSafetyAdvancedGrid label {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 700;
  user-select: none;
}

.newRunSafetyAdvancedGrid input {
  width: 18px;
  height: 18px;
}

.newRunSafetyExtra {
  border-top: 1px dashed rgba(148, 163, 184, 0.35);
  padding-top: 10px;
  display: grid;
  gap: 8px;
}

.newRunSafetyWarn {
  border-top: 1px solid rgba(148, 163, 184, 0.35);
  padding-top: 10px;
  display: grid;
  gap: 8px;
}

.newRunSafetyOptIn {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 700;
  user-select: none;
}

.newRunSafetyOptIn input {
  width: 18px;
  height: 18px;
}

.inlineBtn {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
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
  bottom: max(26px, env(safe-area-inset-bottom));
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
  right: max(16px, env(safe-area-inset-right));
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

:deep(.secDrawerOverlay) {
  position: fixed;
  inset: 0;
  background: var(--overlay-drawer);
  backdrop-filter: blur(2px);
  z-index: 200;
}

:deep(.secDrawer) {
  position: fixed;
  top: max(16px, env(safe-area-inset-top));
  right: 16px;
  bottom: max(16px, env(safe-area-inset-bottom));
  width: min(980px, calc(100vw - 32px));
  background: var(--bg-panel);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--border-color);
  overflow: hidden;
  display: grid;
  grid-template-rows: auto 1fr;
}

:deep(.secDrawer.secDrawerSecretary) {
  width: min(1100px, calc(100vw - 32px));
}

:deep(.secDrawer.wide) {
  width: min(980px, calc(100vw - 32px));
}

:deep(.secResizeHandle) {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 10px;
  cursor: col-resize;
  z-index: 2;
}

:deep(.secResizeHandle)::after {
  content: "";
  position: absolute;
  left: 4px;
  top: 16px;
  bottom: 16px;
  width: 2px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.6);
  opacity: 0;
  transition: opacity 0.15s ease;
}

:deep(.secResizeHandle:hover)::after,
:deep(.secResizeHandle.active)::after {
  opacity: 1;
}

.feedCoach {
  position: fixed;
  right: 88px;
  bottom: max(28px, env(safe-area-inset-bottom));
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-lg);
  border-radius: 14px;
  padding: 10px 12px;
  display: grid;
  gap: 10px;
  z-index: 260;
  max-width: min(360px, calc(100vw - 120px));
}

.feedCoachText {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-main);
}

.feedCoachActions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:deep(.secDrawerHeader) {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

:deep(.secDrawerTitle) {
  font-weight: 800;
  font-size: 14px;
  color: var(--text-main);
}

:deep(.secTabs) {
  display: flex;
  gap: 6px;
  flex: 1;
  justify-content: center;
}

:deep(.secTab) {
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
  position: relative;
}

:deep(.secTab.active) {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

:deep(.secTabBadge) {
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

:deep(.secDrawerBody) {
  padding: 14px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

:deep(.secOverview) {
  display: grid;
  gap: 20px;
  overflow: auto;
  min-height: 0;
  flex: 1;
}

:deep(.secChatView) {
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

:deep(.secAttentionHint) {
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

:deep(.secAttentionHint .text) {
  flex: 1;
}

.secFeed {
  display: grid;
  gap: 12px;
  height: calc(100vh - 170px);
  height: calc(100dvh - 170px);
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

.feedConn {
  font-size: 12px;
  font-weight: 900;
  color: var(--text-sub);
  background: var(--bg-subtle);
  border: 1px solid rgba(148, 163, 184, 0.25);
  padding: 6px 10px;
  border-radius: 999px;
}

.feedConn.bad {
  border-color: rgba(245, 158, 11, 0.35);
  color: rgba(245, 158, 11, 0.95);
  background: rgba(245, 158, 11, 0.08);
}

.feedReconnect {
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 900;
  border-radius: 999px;
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

.feedHint {
  font-size: 12px;
  font-weight: 900;
  color: var(--text-sub);
  background: var(--bg-subtle);
  border: 1px solid rgba(148, 163, 184, 0.25);
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

/* Narrow Sessions panel handling (container query) */
@container (max-width: 420px) {
  .workspaceBar {
    flex-wrap: wrap;
    align-items: stretch;
  }

  .workspaceLeft {
    flex: 1 1 100%;
  }

  .workspaceTitle {
    display: none;
  }

  .workspaceLeft select {
    width: 100%;
    min-width: 0;
  }

  .workspaceBar button {
    flex: 1 1 auto;
    padding-left: 10px;
    padding-right: 10px;
    white-space: nowrap;
  }

  .pinnedWorkspaces {
    flex-direction: column;
  }

  .pinnedItem {
    width: 100%;
    flex: 1 1 100%;
  }

  .pinnedBtn {
    max-width: none;
    flex: 1 1 auto;
    min-width: 0;
  }

  .pinnedX {
    width: 44px;
    padding-left: 0;
    padding-right: 0;
  }

  .pinnedEdit {
    width: 44px;
    padding-left: 0;
    padding-right: 0;
  }
}

:deep(.modalOverlay) {
  position: fixed;
  inset: 0;
  background: var(--overlay-modal);
  backdrop-filter: blur(4px);
  display: grid;
  place-items: center;
  padding: 24px;
  z-index: 999;
}

:deep(.modal), :deep(.settingsModal) {
  background: var(--bg-panel);
  border-radius: 24px;
  border: 1px solid var(--border-color);
  box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1);
  overflow: hidden;
  animation: popIn 0.2s ease-out;
}

:deep(.modal) {
  width: min(860px, 95vw);
  height: min(600px, 90vh);
  display: grid;
  grid-template-rows: auto 1fr auto;
}

.smallModal {
  width: min(520px, 95vw);
  height: auto;
  max-height: 90vh;
}

.smallModal .modalBody {
  overflow: auto;
}

.settingsModal {
  width: min(760px, 95vw);
  height: min(600px, 90vh);
}

:deep(.headerMiniBtn) {
  margin-left: 10px;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text);
  font-weight: 700;
}

:deep(.headerMiniBtn:disabled) {
  opacity: 0.6;
  cursor: not-allowed;
}

:deep(.skillsModal) {
  width: min(980px, 95vw);
  height: min(660px, 90vh);
}

.toolsModal {
  width: min(980px, 95vw);
  height: min(660px, 90vh);
}

.toolsBody {
  padding-top: 12px;
}

.toolsSplit {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 14px;
  height: 100%;
  min-height: 0;
}

.toolsList {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  overflow: auto;
  padding: 8px;
  min-height: 0;
}

.toolsItem {
  width: 100%;
  text-align: left;
  padding: 10px 10px;
  border-radius: 12px;
  border: 1px solid transparent;
  background: transparent;
}

.toolsItem:hover {
  background: var(--bg-subtle);
}

.toolsItem.active {
  border-color: rgba(20, 184, 166, 0.35);
  background: rgba(20, 184, 166, 0.08);
}

.toolsEditor {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  overflow: auto;
  padding: 12px;
  min-height: 0;
}

.toolsEditorGrid {
  display: grid;
  gap: 12px;
}

.toolsEditorGrid input,
.toolsEditorGrid textarea,
.toolsEditorGrid select {
  width: 100%;
}

:deep(.skillsBody) {
  padding-top: 12px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

:deep(.skillsPageBody) {
  flex: 1;
  min-height: 0;
}

:deep(.skillsMeta) {
  display: grid;
  gap: 6px;
  margin-top: 8px;
}

:deep(.skillsMetaDetails) {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 12px;
  background: var(--bg-subtle);
}

:deep(.skillsMetaDetails > summary) {
  cursor: pointer;
  list-style: none;
  font-weight: 800;
  color: var(--text-main);
}

:deep(.skillsMetaDetails > summary::-webkit-details-marker) {
  display: none;
}

:deep(.skillsMetaDetails[open] > summary) {
  margin-bottom: 8px;
}

:deep(.skillsToolbar) {
  display: flex;
  gap: 10px;
  align-items: center;
  margin: 10px 0 14px;
}

:deep(.skillsToolbar input) {
  width: 100%;
  flex: 1;
  min-width: 0;
}

:deep(.skillsLimit) {
  display: flex;
  gap: 8px;
  align-items: center;
  white-space: nowrap;
}

:deep(.skillsPager) {
  display: flex;
  gap: 8px;
  align-items: center;
}

:deep(.skillsTable) {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  overflow: hidden;
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-rows: auto 1fr;
}

:deep(.skillsHead) {
  display: grid;
  grid-template-columns: 1.4fr 1fr 1fr 1fr;
  gap: 12px;
  padding: 10px 12px;
  background: var(--bg-subtle);
  font-weight: 800;
}

:deep(.skillsRows) {
  overflow: auto;
  min-height: 0;
}

:deep(.skillsRow) {
  display: grid;
  grid-template-columns: 1.4fr 1fr 1fr 1fr;
  gap: 12px;
  padding: 12px;
  border-top: 1px solid var(--border-color);
  align-items: center;
}

:deep(.skillsName) {
  min-width: 0;
}

:deep(.skillsName .mono) {
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.skillsCell) {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}

:deep(.skillActionBtn) {
  opacity: 0;
  pointer-events: none;
  transform: translateY(-1px);
  transition: opacity 0.15s ease, transform 0.15s ease;
}

:deep(.skillsRow:hover .skillActionBtn),
:deep(.skillsRow:focus-within .skillActionBtn) {
  opacity: 1;
  pointer-events: auto;
  transform: translateY(0);
}

@media (max-width: 720px) {
  :deep(.skillActionBtn) {
    opacity: 1;
    pointer-events: auto;
    transform: none;
  }
}

:deep(.skillStatus) {
  min-width: 72px;
  text-align: center;
}

:deep(.pill.skillStatus.ok) {
  border-color: rgba(16, 185, 129, 0.45);
  color: rgb(16, 185, 129);
}

:deep(.pill.skillStatus.warn) {
  border-color: rgba(245, 158, 11, 0.45);
  color: rgb(245, 158, 11);
}

:deep(.pill.skillStatus.partial) {
  border-color: rgba(59, 130, 246, 0.45);
  color: rgb(59, 130, 246);
}

:deep(.pill.skillStatus.muted) {
  opacity: 0.75;
}

:deep(.pill.skillStatus.dim) {
  opacity: 0.6;
}

:deep(.skillsPageWrap) {
  display: grid;
  grid-template-columns: minmax(420px, 600px) 1fr;
  gap: 16px;
  padding: 20px;
  min-height: 0;
  overflow: hidden;
}

:deep(.skillsLeftCol) {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 0;
  overflow: hidden;
}

:deep(.skillsVersionsCard) {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: var(--bg-panel);
  padding: 16px;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

:deep(.skillsVersionsHeader) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}

:deep(.skillsVersionsTitle) {
  font-weight: 800;
}

:deep(.skillsVersionsMeta) {
  display: grid;
  gap: 6px;
  margin-bottom: 12px;
}

:deep(.skillsVersionsCreate) {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 10px;
  margin-bottom: 10px;
}

:deep(.skillsVersionsCreate input) {
  width: 100%;
  min-width: 0;
}

:deep(.skillsVersionsList) {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border-top: 1px solid var(--border-color);
  margin-top: 12px;
  padding-top: 12px;
}

:deep(.skillsGovernanceCard) {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: var(--bg-panel);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow: auto;
}

:deep(.skillsGovernanceHeader) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

:deep(.skillsGovernanceHeaderBtns) {
  display: flex;
  gap: 10px;
  align-items: center;
}

:deep(.skillsGovernanceTitle) {
  font-weight: 800;
}

:deep(.skillsGovernanceTools) {
  display: grid;
  gap: 8px;
}

:deep(.skillsGovIntro) {
  line-height: 1.4;
}

:deep(.skillsGovDetails) {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 12px;
  background: var(--bg-subtle);
}

:deep(.skillsGovDetails > summary) {
  cursor: pointer;
  list-style: none;
  font-weight: 800;
  color: var(--text-main);
}

:deep(.skillsGovDetails > summary::-webkit-details-marker) {
  display: none;
}

:deep(.skillsGovDetails[open] > summary) {
  margin-bottom: 10px;
}

:deep(.skillsGovTabs) {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 6px 0 10px;
}

:deep(.skillsGovPrimary) {
  border: 1px solid rgba(45, 212, 191, 0.18);
  border-radius: 16px;
  padding: 12px;
  background: linear-gradient(
    180deg,
    rgba(20, 184, 166, 0.12) 0%,
    rgba(15, 23, 42, 0.08) 70%,
    rgba(15, 23, 42, 0.02) 100%
  );
  display: grid;
  gap: 10px;
}

:deep(.skillsGovPrimaryHeader) {
  display: grid;
  gap: 4px;
}

:deep(.skillsGovPrimaryTitle) {
  font-weight: 900;
  letter-spacing: 0.2px;
}

:deep(.skillsGovTab) {
  border: 1px solid rgba(148, 163, 184, 0.22);
  background: var(--bg-panel);
  color: var(--text-sub);
  font-weight: 800;
  font-size: 12px;
  border-radius: 999px;
  padding: 6px 10px;
  cursor: pointer;
}

:deep(.skillsGovTab:hover) {
  border-color: rgba(45, 212, 191, 0.35);
  color: var(--color-primary);
}

:deep(.skillsGovTab.active) {
  border-color: rgba(20, 184, 166, 0.45);
  background: rgba(20, 184, 166, 0.08);
  color: var(--color-primary);
}

:deep(.skillsGovOp) {
  display: grid;
  gap: 12px;
}

:deep(.skillsGovToolRow) {
  display: grid;
  grid-template-columns: auto auto 1fr;
  gap: 10px;
  align-items: center;
}

:deep(.skillsGovToolRoots) {
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.skillsGovSection) {
  border-top: 1px solid var(--border-color);
  padding-top: 12px;
}

:deep(.skillsGovSectionTitle) {
  font-weight: 800;
  margin-bottom: 8px;
}

:deep(.skillsGovFields) {
  display: grid;
  gap: 10px;
}

:deep(.skillsGovField) {
  display: grid;
  gap: 6px;
}

:deep(.skillsGovFieldLabel) {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-main);
}

:deep(.skillsGovReq) {
  color: rgb(244, 63, 94);
  font-weight: 900;
}

:deep(.skillsGovField input),
:deep(.skillsGovField select) {
  width: 100%;
  min-width: 0;
}

:deep(.skillsGovRow) {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
  align-items: center;
}

:deep(.skillsGovActions) {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 2px;
}

:deep(.skillsGovSecondaryBtn) {
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: rgba(2, 6, 23, 0.15);
  color: var(--text-main);
  font-weight: 800;
  border-radius: 12px;
  padding: 8px 10px;
  white-space: nowrap;
}

:deep(.skillsGovSecondaryBtn:hover) {
  border-color: rgba(45, 212, 191, 0.35);
  color: var(--color-primary);
}

:deep(.skillsGovSecondaryBtn:disabled) {
  opacity: 0.55;
  cursor: not-allowed;
}

:deep(.skillsGovPrimaryBtn) {
  min-width: 112px;
}

:deep(.skillsGovCheckbox) {
  display: flex;
  gap: 8px;
  align-items: center;
  white-space: nowrap;
  border: 1px solid rgba(148, 163, 184, 0.2);
  background: rgba(2, 6, 23, 0.12);
  padding: 6px 10px;
  border-radius: 999px;
}

:deep(.skillsGovCandidates) {
  display: grid;
  gap: 8px;
  margin: 8px 0;
  padding: 10px 12px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.12);
}

:deep(.skillsGovOnboardingList) {
  display: grid;
  gap: 8px;
  margin-top: 10px;
}

:deep(.skillsGovOnboardingRow) {
  display: grid;
  grid-template-columns: auto auto 1fr;
  gap: 10px;
  align-items: center;
}

:deep(.skillsVersionRow) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--border-color);
}

:deep(.skillsVersionRow:first-child) {
  border-top: 0;
}

:deep(.skillsVersionMain) {
  min-width: 0;
}

:deep(.skillsVersionMain .mono) {
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.skillsVersionRight) {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

@keyframes popIn {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}

:deep(.modalHeader) {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

:deep(.modalFooter) {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px;
  border-top: 1px solid var(--border-color);
  background: var(--bg-subtle);
}

:deep(.modalTitle) {
  font-weight: 700;
  font-size: 16px;
  color: var(--text-main);
}

.tabDivider {
  width: 1px;
  height: 24px;
  background: var(--border-color);
  margin: 0 6px;
  opacity: 0.8;
}

.rawBox {
  margin: 0;
  padding: 14px 16px;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.htmlPreviewFrame {
  width: 100%;
  height: 100%;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: var(--bg-panel);
}

:deep(.iconBtn) {
  border: none;
  background: transparent;
  padding: 8px;
  color: var(--text-sub);
  border-radius: 50%;
}
:deep(.iconBtn:hover) {
  background: var(--bg-subtle);
  color: var(--text-main);
}

:deep(.modalBody), :deep(.settingsBody), :deep(.dirModalBody) {
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

.fileModal {
  width: min(1100px, 96vw);
  height: min(760px, 92vh);
}

.filesModal {
  width: min(1180px, 96vw);
  height: min(780px, 92vh);
}

.fileModalBody {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.filesModalBody {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.filesTopRow {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.filesRootPath {
  font-size: 12px;
  color: var(--text-sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.filesSplit {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: var(--sidebar-width, 340px) 12px 1fr;
  gap: 0;
}

.filesResizer {
  width: 100%;
  cursor: col-resize;
  background: transparent;
  display: flex;
  justify-content: center;
  transition: background 0.2s;
}

.filesResizer:hover,
.filesResizer.resizing {
  background: rgba(148, 163, 184, 0.25);
}

.filesTreePane,
.filesEditorPane {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: rgba(0, 0, 0, 0.02);
  padding: 10px;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

:global(:root[data-theme="dark"]) .filesTreePane,
:global(:root[data-theme="dark"]) .filesEditorPane {
  background: rgba(255, 255, 255, 0.03);
}

.filesTreeActions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.filesTreeList {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 4px 2px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.filesNode {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  border-radius: 10px;
  padding: 8px 10px;
}

.filesNode:hover {
  background: rgba(13, 148, 136, 0.08);
}

.filesNode.active {
  background: var(--color-primary-bg);
  border: 1px solid rgba(13, 148, 136, 0.35);
}

.filesNodeTwisty {
  width: 14px;
  color: var(--text-sub);
  flex: 0 0 auto;
}

.filesNodeIcon {
  width: 18px;
  flex: 0 0 auto;
}

.filesNodeName {
  flex: 1 1 auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.filesNodeMeta {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--text-sub);
}

.filesEditorHeader {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.filesEditorPath {
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--text-sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1 1 auto;
  min-width: 0;
}

.filesEditorEdit {
  flex: 1;
  min-height: 0;
  display: flex;
}

.filesEditorEdit textarea {
  flex: 1;
  min-height: 0;
  resize: none;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
}

@media (max-width: 860px) {
  .filesSplit {
    grid-template-columns: 1fr;
  }
  .filesTreePane {
    max-height: 240px;
  }
  .filesResizer {
    display: none;
  }
}

.fileMetaRow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.fileMetaLeft {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.fileMetaActions {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.filePreviewBox,
.fileCodeBox {
  flex: 1;
  min-height: 0;
}

.fileCodeBox {
  white-space: normal;
  padding: 0;
}

.fileCodeBox pre {
  margin: 0;
  padding: 14px 16px;
  min-height: 100%;
  overflow: auto;
  background: transparent;
}

.fileCodeBox code {
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.resultBox :deep(.hljs) {
  color: var(--text-main);
}

.resultBox :deep(.hljs-comment),
.resultBox :deep(.hljs-quote) {
  color: var(--text-sub);
  font-style: italic;
}

.resultBox :deep(.hljs-keyword),
.resultBox :deep(.hljs-selector-tag),
.resultBox :deep(.hljs-subst) {
  color: #2563eb;
}

.resultBox :deep(.hljs-string),
.resultBox :deep(.hljs-title),
.resultBox :deep(.hljs-section),
.resultBox :deep(.hljs-attribute) {
  color: #0f766e;
}

.resultBox :deep(.hljs-number),
.resultBox :deep(.hljs-literal),
.resultBox :deep(.hljs-symbol),
.resultBox :deep(.hljs-bullet) {
  color: #7c3aed;
}

.resultBox :deep(.hljs-meta),
.resultBox :deep(.hljs-doctag) {
  color: #be185d;
}

:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-keyword),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-selector-tag),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-subst) {
  color: #60a5fa;
}

:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-string),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-title),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-section),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-attribute) {
  color: #2dd4bf;
}

:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-number),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-literal),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-symbol),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-bullet) {
  color: #a78bfa;
}

:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-meta),
:global(:root[data-theme="dark"]) .resultBox :deep(.hljs-doctag) {
  color: #f472b6;
}

:deep(.modalError) {
  border: 1px solid rgba(239, 68, 68, 0.25);
  background: rgba(239, 68, 68, 0.08);
  color: var(--text-main);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  font-size: 13px;
  white-space: pre-wrap;
}

.pill.warn {
  background: rgba(245, 158, 11, 0.16);
  color: #b45309;
  border-color: rgba(245, 158, 11, 0.25);
}

:deep(.pill.low) {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
  border-color: rgba(16, 185, 129, 0.25);
}

:global(:root[data-theme="dark"]) :deep(.pill.low) {
  color: #6ee7b7;
}

:deep(.pill.safety) {
  background: rgba(59, 130, 246, 0.12);
  color: var(--text-sub);
  border-color: rgba(59, 130, 246, 0.22);
  text-transform: none;
  letter-spacing: 0;
  font-weight: 700;
}

:global(:root[data-theme="dark"]) .pill.warn {
  color: #fbbf24;
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

.settingsToggleRow {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--text-main);
}

.foremanToast {
  position: fixed;
  right: 18px;
  bottom: calc(92px + env(safe-area-inset-bottom));
  z-index: 999;
  max-width: min(520px, calc(100vw - 36px));
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-lg);
  border-radius: 14px;
  padding: 12px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.foremanText {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 13px;
  color: var(--text-main);
}

.foremanActions {
  flex: 0 0 auto;
  display: inline-flex;
  gap: 8px;
}

@media (max-width: 520px) {
  .foremanToast {
    left: 18px;
    right: 18px;
    bottom: calc(86px + env(safe-area-inset-bottom));
  }
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
  gap: 10px;
  flex: 1;
  min-height: 0;
  overflow: visible;
  padding: 16px; /* match panel content padding (compact) */
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

.pinnedBtn .mono,
.pinnedBtn .pinName,
.pinnedBtn .pinSub {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pinnedBtn .pinName {
  font-weight: 800;
}

.pinnedBtn .pinSub {
  font-size: 11px;
  opacity: 0.75;
}

.pinnedBtn.active .pinSub {
  opacity: 0.9;
}

.pinnedEdit {
  border: none;
  border-left: 1px solid var(--border-color);
  background: transparent;
  padding: 6px 10px;
  cursor: pointer;
  color: var(--text-sub);
}

.pinnedEdit:hover {
  color: var(--color-primary);
  background: var(--bg-subtle);
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
  padding: 10px 12px;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 2px 4px rgba(0,0,0,0.03);
  cursor: pointer;
  position: relative;
  overflow: visible;
  z-index: 0;
}

.row:focus-visible {
  outline: 2px solid rgba(20, 184, 166, 0.8);
  outline-offset: 2px;
}

.row.deleted {
  opacity: 0.7;
}

.row::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  border-top-left-radius: var(--radius-md);
  border-bottom-left-radius: var(--radius-md);
  background: transparent;
  transition: background 0.2s;
}

.row:hover {
  transform: translateY(-2px);
  z-index: 6;
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
  margin-bottom: 6px;
}

.rowTopLeft {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.rowTopRight {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}

.rowSub {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.rowWorkdir {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 42%;
  font-size: 11px;
  color: var(--text-sub);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rowPrompt {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 12px;
  color: var(--text-main);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rowName {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-main);
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rowMoreBtn {
  width: 30px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(15, 23, 42, 0.12);
  color: var(--text-main);
  font-weight: 900;
  font-size: 16px;
  cursor: pointer;
}

.rowMorePopup {
  position: fixed;
  width: 180px;
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: 8px;
  display: grid;
  gap: 8px;
  z-index: 300;
}

.rowMorePopup button {
  width: 100%;
  text-align: left;
}

.menuOverlay {
  position: fixed;
  inset: 0;
  background: transparent;
  z-index: 290;
}

:deep(.mono) {
  font-family: var(--font-mono);
  font-size: 0.9em;
}

:deep(.pill) {
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

.pill.deleted {
  background: rgba(148, 163, 184, 0.14);
  color: var(--text-sub);
  border-color: rgba(148, 163, 184, 0.25);
  text-transform: none;
  letter-spacing: 0;
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

.detailHeader.compact {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  margin-bottom: 10px;
}

.detailTop {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.detailHeader.compact .detailTop {
  width: 100%;
  flex-wrap: nowrap;
}

.detailTopLeft {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.detailHeader.compact .detailTopLeft {
  flex-wrap: nowrap;
  overflow: hidden;
  white-space: nowrap;
}

.detailSid {
  font-weight: 900;
  font-size: 14px;
}

.detailName {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-main);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detailPrompt {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-sub);
  padding-left: 10px;
  margin-left: 2px;
  border-left: 1px solid rgba(148, 163, 184, 0.25);
}

.detailPrompt.running {
  flex: 0 1 auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-left: none;
  padding: 4px 10px;
  margin-left: 8px;
  border: 1px solid rgba(20, 184, 166, 0.35);
  background: rgba(20, 184, 166, 0.12);
  color: var(--text-main);
  font-weight: 800;
  border-radius: 999px;
}

.detailPrompt.running::before {
  content: "⏳";
  font-size: 12px;
  opacity: 0.9;
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

.detailMiniBtn {
  cursor: pointer;
  transition: all 0.2s;
}

.detailMiniBtn:hover {
  border-color: rgba(20, 184, 166, 0.6);
  box-shadow: 0 0 0 2px rgba(20, 184, 166, 0.2);
  color: var(--color-primary);
}

.detailMiniBtn:active {
  transform: translateY(1px);
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

.detailMore.compact {
  border-top: 0;
  padding-top: 0;
  position: relative;
}

.detailMore.compact summary {
  width: 34px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(15, 23, 42, 0.18);
  color: var(--text-main);
  font-weight: 900;
  font-size: 16px;
}

.detailMore.compact[open] summary {
  border-color: rgba(20, 184, 166, 0.6);
  box-shadow: 0 0 0 2px rgba(20, 184, 166, 0.2);
}

.detailMorePopup {
  position: absolute;
  right: 0;
  top: calc(100% + 8px);
  width: min(560px, 86vw);
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: 12px;
  z-index: 10;
}

.detailPopupWorkdir {
  font-size: 12px;
  color: var(--text-sub);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-bottom: 10px;
}

.detailMoreGrid {
  margin-top: 10px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 14px;
  font-size: 13px;
}

.detailMoreActions {
  grid-column: 1 / -1;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}

.detailMoreActions button {
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 800;
  border-radius: 999px;
}

.detailMoreGrid .full {
  grid-column: 1 / -1;
}

.blockedHint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  border: 1px solid rgba(249, 115, 22, 0.35);
  background: rgba(249, 115, 22, 0.08);
  color: var(--text-main);
  margin-bottom: 12px;
}

.blockedHint .text {
  font-size: 13px;
  color: var(--text-main);
}

.blockedHint .actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}

.blockedHint .actions button {
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 800;
  border-radius: 999px;
}

.pill.acceptance {
  border: 1px solid rgba(59, 130, 246, 0.35);
  background: rgba(59, 130, 246, 0.12);
  color: var(--text-main);
}

.acceptanceHint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  border: 1px solid rgba(59, 130, 246, 0.28);
  background: rgba(59, 130, 246, 0.08);
  color: var(--text-main);
  margin-bottom: 12px;
}

.acceptanceHint .text {
  font-size: 13px;
  color: var(--text-main);
}

.acceptanceHint .actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}

.acceptanceHint .actions button {
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 800;
  border-radius: 999px;
}

.acceptanceReport {
  margin-bottom: 12px;
}

@container (max-width: 540px) {
  .detailHeader.compact .detailTopLeft .pill.kind {
    display: none;
  }
}

@container (max-width: 460px) {
  .detailHeader.compact .detailMini {
    display: none;
  }
}

.resumeBar {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
}

.resumeRow {
  display: grid;
  grid-template-columns: 1fr auto auto auto;
  gap: 10px;
  align-items: start;
}

.resumeRow textarea {
  min-height: 0;
}

.resumeSafetyControls {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-height: 40px;
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(15, 23, 42, 0.12);
  color: var(--text-main);
  opacity: 0.95;
}

.resumeSafetyLabel {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 800;
  user-select: none;
}

.resumeSafetyLabel select {
  height: 28px;
}

.resumeSafetyExtra {
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: var(--radius-md);
  background: rgba(15, 23, 42, 0.08);
  padding: 10px 12px;
  display: grid;
  gap: 8px;
}

.resumeSafetyExtraGrid {
  display: grid;
  gap: 8px;
}

.resumeSafetyExtraGrid label {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 700;
  user-select: none;
  flex-wrap: wrap;
}

.resumeSafetyExtraGrid input {
  width: 18px;
  height: 18px;
}

.resumeSafetyGrid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-items: end;
}

.resumeSafetyWarn {
  border: 1px solid rgba(239, 68, 68, 0.35);
  border-radius: var(--radius-md);
  background: rgba(239, 68, 68, 0.06);
  padding: 10px 12px;
  display: grid;
  gap: 8px;
}

.resumeSafetyOptIn {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 800;
  user-select: none;
}

.resumeSafetyOptIn input {
  width: 18px;
  height: 18px;
}

.resumeToggle {
  padding: 0 12px;
  height: 40px;
  font-size: 16px;
  font-weight: 900;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(15, 23, 42, 0.12);
  color: var(--text-main);
  opacity: 0.85;
}

.resumeToggle:hover {
  opacity: 1;
}

:deep(.tinyHint) {
  font-size: 12px;
  color: var(--text-sub);
}

.confirmText {
  font-size: 14px;
  color: var(--text-main);
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

.runsModal {
  width: min(920px, 95vw);
  height: min(720px, 92vh);
}

.runsModalBody {
  overflow: auto;
}

.runsModalList {
  max-height: none;
  padding: 12px;
}

.runRow {
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 10px 12px;
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
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 6px;
}

.runTopLeft {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--text-sub);
  min-width: 0;
}

.runId {
  font-weight: 900;
  color: var(--text-main);
}

.runTime {
  opacity: 0.8;
  white-space: nowrap;
}

.runBottom {
  font-size: 14px;
  font-weight: 600;
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

  .resultPanel, .logsPanel, .tracePanel {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .runWorkspaceBanner {
    border: 1px dashed var(--border-color);
    border-radius: var(--radius-md);
    padding: 10px 12px;
    background: var(--bg-panel);
  }

  .runWorkspaceBannerActions {
    display: flex;
    gap: 10px;
    margin-top: 8px;
    flex-wrap: wrap;
  }

  .traceBox {
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    padding: 14px 16px;
  background: var(--bg-panel);
  flex: 1;
  overflow: auto;
  display: grid;
  gap: 10px;
}

.traceRow {
  display: grid;
  grid-template-columns: 54px 1fr;
  gap: 12px;
  align-items: start;
}

.traceRow .k {
  opacity: 0.7;
  font-weight: 700;
}

.traceEnv {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
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

.resultBox.markdown {
  white-space: normal;
}

.resultBox.markdown :deep(p) {
  margin: 0 0 10px;
}

.resultBox.markdown :deep(h1),
.resultBox.markdown :deep(h2),
.resultBox.markdown :deep(h3) {
  margin: 14px 0 10px;
  line-height: 1.25;
}

.resultBox.markdown :deep(h1) { font-size: 20px; }
.resultBox.markdown :deep(h2) { font-size: 17px; }
.resultBox.markdown :deep(h3) { font-size: 15px; }

.resultBox.markdown :deep(pre) {
  margin: 10px 0;
  padding: 12px;
  background: var(--bg-subtle);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
}

.resultBox.markdown :deep(code) {
  font-family: var(--font-mono);
  font-size: 12px;
}

.resultBox.markdown :deep(p code),
.resultBox.markdown :deep(li code) {
  background: var(--bg-subtle);
  border: 1px solid rgba(148, 163, 184, 0.22);
  padding: 1px 6px;
  border-radius: 8px;
}

.resultBox.markdown :deep(code.fileRef) {
  cursor: pointer;
  border-color: rgba(45, 212, 191, 0.45);
}

.resultBox.markdown :deep(code.fileRef:hover) {
  color: var(--color-primary);
  text-decoration: underline;
}

.resultBox.markdown :deep(a) {
  color: var(--color-primary);
  text-decoration: none;
}

.resultBox.markdown :deep(a:hover) {
  text-decoration: underline;
}

.resultBox.markdown :deep(table) {
  width: max-content;
  max-width: 100%;
  display: block;
  overflow-x: auto;
  border-collapse: collapse;
  margin: 10px 0;
  font-size: 13px;
}

.resultBox.markdown :deep(th),
.resultBox.markdown :deep(td) {
  border: 1px solid var(--border-color);
  padding: 8px 10px;
  text-align: left;
  vertical-align: top;
}

.resultBox.markdown :deep(th) {
  background: var(--bg-subtle);
  font-weight: 800;
}

.resultBox.markdown :deep(blockquote) {
  margin: 10px 0;
  padding: 10px 12px;
  border-left: 3px solid var(--color-primary);
  background: var(--bg-subtle);
  color: var(--text-main);
  border-radius: 10px;
}

.resultBox.markdown :deep(img) {
  max-width: 100%;
  height: auto;
}

.resultBox.markdown :deep(.mermaid) {
  background: var(--bg-subtle);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px;
  overflow: auto;
  margin: 10px 0;
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
  background: var(--bg-panel);
  color: var(--text-main);
  flex: 1;
  min-height: 0;
  overflow: auto;
  font-size: 12px;
  line-height: 1.45;
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.08);
}

.logbox.raw {
  font-family: var(--font-mono);
}

.logbox.pretty {
  padding: 8px 10px;
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
  color: var(--text-sub);
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

.logEvent {
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
}

.logEvent:last-child {
  border-bottom: none;
}

.logEventSummary {
  display: grid;
  grid-template-columns: 64px 74px 1fr;
  gap: 10px;
  align-items: start;
  padding: 6px 0;
  cursor: pointer;
  list-style: none;
}

.logEventSummary::-webkit-details-marker {
  display: none;
}

.logEventSummary::marker {
  content: "";
}

.logEvent[open] .logEventSummary {
  background: rgba(148, 163, 184, 0.08);
  border-radius: 10px;
  padding-left: 8px;
  padding-right: 8px;
}

.logSummary {
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.logEventBody {
  padding: 8px 0 12px;
}

.logDetail {
  border: 1px solid var(--border-color);
  background: var(--bg-subtle);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  overflow: auto;
  max-height: 320px;
  margin: 0 0 0 148px;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.55;
}

.logbox.pretty :deep(.hljs) {
  color: var(--text-main);
  background: transparent;
  padding: 0;
}

@media (max-width: 720px) {
  .logEventSummary {
    grid-template-columns: 60px 70px 1fr;
  }

  .logLine {
    grid-template-columns: 60px 70px 1fr;
  }

  .logDetail {
    margin-left: 0;
  }
}

.logbox.pretty :deep(.hljs-comment),
.logbox.pretty :deep(.hljs-quote) {
  color: var(--text-sub);
  font-style: italic;
}

.logbox.pretty :deep(.hljs-keyword),
.logbox.pretty :deep(.hljs-selector-tag),
.logbox.pretty :deep(.hljs-subst) {
  color: #2563eb;
}

.logbox.pretty :deep(.hljs-string),
.logbox.pretty :deep(.hljs-title),
.logbox.pretty :deep(.hljs-section),
.logbox.pretty :deep(.hljs-attribute) {
  color: #0f766e;
}

.logbox.pretty :deep(.hljs-number),
.logbox.pretty :deep(.hljs-literal),
.logbox.pretty :deep(.hljs-symbol),
.logbox.pretty :deep(.hljs-bullet) {
  color: #7c3aed;
}

.logbox.pretty :deep(.hljs-meta),
.logbox.pretty :deep(.hljs-doctag) {
  color: #be185d;
}

:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-keyword),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-selector-tag),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-subst) {
  color: #60a5fa;
}

:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-string),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-title),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-section),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-attribute) {
  color: #2dd4bf;
}

:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-number),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-literal),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-symbol),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-bullet) {
  color: #a78bfa;
}

:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-meta),
:global(:root[data-theme="dark"]) .logbox.pretty :deep(.hljs-doctag) {
  color: #f472b6;
}

:deep(.secretary) {
  display: grid;
  gap: 20px;
  height: auto; /* Allow auto height now that we are sticky */
  grid-template-rows: auto 1fr auto;
}

:deep(.secretaryCards) {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

:deep(.secCard) {
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

:deep(.secCard:hover) {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(13, 148, 136, 0.15);
}

:deep(.secK) {
  font-size: 11px;
  color: var(--text-sub);
  text-transform: uppercase;
  font-weight: 700;
  letter-spacing: 0.05em;
}

:deep(.secV) {
  font-size: 24px;
  font-weight: 800;
  color: var(--color-primary);
  margin-top: 6px;
}

:deep(.secSection) {
  display: grid;
  gap: 10px;
}

:deep(.secSectionTitle) {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-main);
}

:deep(.secSectionTitleRow) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

:deep(.secSectionControls) {
  display: flex;
  align-items: center;
  gap: 10px;
}

:deep(.secScopeSelect) {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: 999px;
}

:deep(.secMiniToggle) {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  background: var(--bg-subtle);
  color: var(--text-sub);
  font-size: 12px;
  font-weight: 800;
  user-select: none;
}

:deep(.secMiniToggle input) {
  width: 14px;
  height: 14px;
}

:deep(.secAutopilotNote) {
  font-size: 12px;
  color: var(--text-sub);
  background: var(--bg-subtle);
  border: 1px solid rgba(148, 163, 184, 0.18);
  padding: 8px 10px;
  border-radius: var(--radius-md);
  overflow-wrap: anywhere;
}

:deep(.secRow) {
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 0;
  transition: all 0.2s;
  display: flex;
  align-items: stretch;
  gap: 10px;
  overflow: hidden;
}

:deep(.secRow:hover) {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: var(--color-primary-bg);
}

:deep(.secRowMain) {
  flex: 1;
  border: none;
  background: transparent;
  padding: 12px;
  text-align: left;
  cursor: pointer;
}

:deep(.secRowActions) {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
}

:deep(.secAction) {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: 999px;
  white-space: nowrap;
}

:deep(.briefing) {
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

:deep(.secChat) {
  flex: 1;
  min-height: 0;
  display: flex;
  border: none;
  border-radius: 0;
  padding: 0;
  background: transparent;
}

:deep(.secChat summary) {
  font-weight: 600;
  color: var(--color-primary);
  cursor: pointer;
}

:deep(.secChat .chat) {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  flex: 1;
}

:deep(.chatControls) {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  margin: 0;
  flex-wrap: wrap;
}

:deep(.chatControls label) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
  color: var(--text-sub);
}

:deep(.chatControls select),
:deep(.chatControls input[type="number"]) {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 8px 10px;
  background: var(--bg-panel);
  font-size: 13px;
}

:deep(.chatControls label.chatToggle) {
  flex-direction: row;
  gap: 8px;
  align-items: center;
  padding-bottom: 8px;
}

:deep(.msgs) {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 12px;
  overflow: auto;
  background: var(--bg-panel);
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

:deep(.msg) {
  padding: 10px 14px;
  border-radius: var(--radius-md);
  margin: 0;
  background: var(--bg-subtle);
  border: 1px solid transparent;
  max-width: min(92%, 820px);
  align-self: flex-start;
}

:deep(.msg.user) {
  background: var(--color-primary-bg);
  color: var(--color-primary-hover);
  align-self: flex-end;
}

:deep(.msg.user .content) {
  color: var(--color-primary-hover);
}

:deep(.msg.streaming) {
  border-style: dashed;
  opacity: 0.9;
}

:deep(.role) {
  font-size: 11px;
  color: var(--text-sub);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 4px;
  font-weight: 700;
}

:deep(.msg .content) {
  font-size: 13px;
  line-height: 1.65;
  color: var(--text-main);
}

:deep(.msg .content.chatMarkdown) {
  white-space: normal;
}

:deep(.msg .content.chatMarkdown p) {
  margin: 0 0 10px;
}

:deep(.msg .content.chatMarkdown pre) {
  margin: 10px 0;
  padding: 12px;
  background: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: auto;
}

:deep(.msg .content.chatMarkdown code) {
  font-family: var(--font-mono);
  font-size: 12px;
}

:deep(.msg .content.chatMarkdown p code),
:deep(.msg .content.chatMarkdown li code) {
  background: var(--bg-panel);
  border: 1px solid rgba(148, 163, 184, 0.22);
  padding: 1px 6px;
  border-radius: 8px;
}

:deep(.msg .content.chatMarkdown code.fileRef) {
  cursor: pointer;
  border-color: rgba(45, 212, 191, 0.45);
}

:deep(.msg .content.chatMarkdown code.fileRef:hover) {
  color: var(--color-primary);
  text-decoration: underline;
}

:deep(.msg .content.chatMarkdown a) {
  color: var(--color-primary);
  text-decoration: none;
}

:deep(.msg .content.chatMarkdown a:hover) {
  text-decoration: underline;
}

:deep(.secChat .input) {
  display: flex;
  gap: 10px;
  align-items: flex-end;
}

:deep(.secChat .input textarea) {
  flex: 1;
  width: auto;
  min-height: 48px;
  max-height: 220px;
  resize: vertical;
}

:deep(.secChat .input button) {
  flex: 0 0 auto;
  min-width: 92px;
  padding-left: 16px;
  padding-right: 16px;
}

@media (max-width: 640px) {
  :deep(.secDrawer) {
    top: 0;
    right: 0;
    bottom: 0;
    width: 100vw;
    border-radius: 0;
  }

  :deep(.secResizeHandle) {
    display: none;
  }

  :deep(.secDrawerHeader) {
    padding-top: calc(12px + env(safe-area-inset-top));
  }

  :deep(.secDrawerBody) {
    padding-bottom: calc(14px + env(safe-area-inset-bottom));
  }
}

@media (max-width: 1300px) {
  .grid {
    grid-template-columns:
      minmax(320px, 1fr)
      minmax(480px, 2fr);
    gap: 16px;
  }
  :deep(.skillsPageWrap) {
    grid-template-columns: 1fr;
  }
  :deep(.secretaryCards) {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (max-width: 900px) {
  .grid {
    grid-template-columns: 1fr;
  }

  /* Disable sticky when single-column */
  .grid > .sessionsPanel:not(.sessionsDrawerPanel) {
    position: static;
    max-height: none;
  }

  .sub {
    display: none;
  }

  button {
    min-height: 40px;
    padding-top: 10px;
    padding-bottom: 10px;
  }

  input,
  select {
    min-height: 40px;
  }

  :deep(.secTab),
  .feedConn,
  .feedReconnect,
  .feedHint,
  .pill {
    min-height: 40px;
    display: inline-flex;
    align-items: center;
  }
}

@media (max-width: 720px) {
  .newRunForm {
    grid-template-columns: 1fr;
  }

  .header {
    padding: 12px 16px;
    align-items: flex-start;
    gap: 10px;
  }

  .headerRight {
    flex: 1;
    min-width: 0;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  }

  .secOrb {
    right: max(16px, env(safe-area-inset-right));
    bottom: max(16px, env(safe-area-inset-bottom));
  }

  :deep(.secDrawer) {
    top: 0;
    right: 0;
    bottom: 0;
    width: 100vw;
    border-radius: 0;
  }

  :deep(.secDrawer.wide) {
    width: 100vw;
  }

  .feedCoach {
    right: 16px;
    left: 16px;
    bottom: 84px;
    max-width: none;
  }
}

@media (max-width: 640px) {
  .newRunOverlay {
    padding: 0;
  }

  .modal.newRunModal {
    width: 100vw;
    height: 100dvh;
    border-radius: 0;
    border: none;
    box-shadow: none;
  }

  .modal.newRunModal .modalHeader {
    padding-top: calc(16px + env(safe-area-inset-top));
  }

  .modal.newRunModal .modalFooter {
    padding-bottom: calc(14px + env(safe-area-inset-bottom));
  }

  .newRunForm {
    padding: 16px;
  }
}

@media (max-width: 520px) {
  .modal.newRunModal .workdirRow {
    grid-template-columns: 1fr;
  }

  .modal.newRunModal .workdirRow button {
    width: 100%;
  }

  .modal.newRunModal .modalFooter {
    justify-content: space-between;
  }

  .modal.newRunModal .modalFooter button {
    flex: 1 1 auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    transition-duration: 0.001ms !important;
    animation-duration: 0.001ms !important;
    animation-iteration-count: 1 !important;
  }

  .row:hover,
  .secRow:hover,
  button.primary:hover:not(:disabled),
  button.dangerBtn:hover:not(:disabled),
  .secOrb:hover {
    transform: none !important;
  }
}
</style>

<script setup lang="ts">
import MarkdownIt from "markdown-it";
import mermaid from "mermaid";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type {
  AuthInfo,
  AuthPatch,
  AuthStatus,
  ChatMessage,
  FSEntry,
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
  fetchFSEntries,
  fetchFSList,
  fetchFSRead,
  fetchFSRoots,
  fetchLogs,
  fetchSystemInfo,
  fetchTasks,
  fsDelete,
  fsMkdir,
  fsWrite,
  resumeTask,
  sendChat,
  sendChatStream,
  updateAuth,
} from "./api";
import { appendChatMessageUnique, sendChatAndReload } from "./chatOps";

const tasks = ref<Map<string, Task>>(new Map());
const selectedTaskId = ref<string>("");
const logsByTask = ref<Map<string, LogEntry[]>>(new Map());

const eventsConnected = ref(true);
const eventsLastEventMs = ref(Date.now());
const eventsLastHeartbeatMs = ref(0);
const eventsLastError = ref("");

const systemInfo = ref<SystemInfo | null>(null);
const chat = ref<ChatMessage[]>([]);

const newWorkerType = ref<WorkerType>("claude-code");
const newWorkdir = ref<string>(".");
const newPrompt = ref<string>("");
const newRunOpen = ref(false);
const newRunPromptEl = ref<HTMLTextAreaElement | null>(null);

const resumePrompt = ref<string>("");
const resumeExpanded = ref(true);
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
const resultPreviewTab = ref<"markdown" | "raw" | "html">("markdown");
const logShowAssistant = ref(true);
const logShowStdout = ref(true);
const logShowStderr = ref(true);
const logShowSystem = ref(true);
const logSearch = ref("");
const sessionSearch = ref("");
const sessionsLimit = ref(40);

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

const filesDirty = computed(
  () => filesFileContent.value !== filesFileOriginal.value,
);

const secretaryOpen = ref(false);
const secretaryView = ref<"chat" | "overview">("chat");

const liveOpen = ref(false);
const liveScope = ref<"current" | "all">("current");
const liveMode = ref<"milestones" | "all">("milestones");
const livePaused = ref(false);
const liveWrap = ref(true);
const liveBoxEl = ref<HTMLDivElement | null>(null);
const liveNowMs = ref(Date.now());
const feedCoachDismissed = ref(false);
const feedCoachOpen = ref(false);

const runsOpen = ref(false);

const isPhone = ref(false);
const sessionsDrawerOpen = ref(false);
const sessionsFiltersOpen = ref(false);

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
const LS_KEY_WORKSPACE_FILTER = "controlccx.workspace_filter.v1";
const LS_KEY_WORKSPACE_FILTERS = "controlccx.workspace_filters.v1";
const LS_KEY_CHAT_BACKEND = "controlccx.chat.backend.v1";
const LS_KEY_CHAT_STREAM = "controlccx.chat.stream.v1";
const LS_KEY_CHAT_MAX_STEPS = "controlccx.chat.max_steps.v1";
const LS_KEY_SECRETARY_VIEW = "controlccx.secretary.view.v1";
const LS_KEY_THEME = "controlccx.theme.v1";
const LS_KEY_FEED_SCOPE = "controlccx.feed.scope.v1";
const LS_KEY_FEED_WRAP = "controlccx.feed.wrap.v1";
const LS_KEY_FEED_MODE = "controlccx.feed.mode.v1";
const LS_KEY_COACH_FEED = "controlccx.coach.feed.v1";

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

  const fs = loadString(LS_KEY_FEED_SCOPE).trim();
  if (fs === "current" || fs === "all") liveScope.value = fs;
  const fm = loadString(LS_KEY_FEED_MODE).trim();
  if (fm === "milestones" || fm === "all") liveMode.value = fm;
  liveWrap.value = loadBool(LS_KEY_FEED_WRAP, true);

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

watch(pinnedWorkspaces, (v) => saveStringArray(LS_KEY_PINNED_WORKSPACES, v), {
  deep: true,
});
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
watch(chatBackend, (v) => saveString(LS_KEY_CHAT_BACKEND, v));
watch(chatStreamEnabled, (v) => saveBool(LS_KEY_CHAT_STREAM, v));
watch(chatMaxSteps, (v) => saveInt(LS_KEY_CHAT_MAX_STEPS, v));
watch(secretaryView, (v) => saveString(LS_KEY_SECRETARY_VIEW, v));
watch(theme, (v) => saveString(LS_KEY_THEME, v));
watch(liveScope, (v) => saveString(LS_KEY_FEED_SCOPE, v));
watch(liveWrap, (v) => saveBool(LS_KEY_FEED_WRAP, v));
watch(liveMode, (v) => saveString(LS_KEY_FEED_MODE, v));
watch(feedCoachDismissed, (v) => saveBool(LS_KEY_COACH_FEED, v));

function desiredOutputTabForTask(t: Task | null): "result" | "logs" {
  if (!t) return "result";
  if (t.status === "running" || t.status === "queued") return "logs";
  return "result";
}

watch(selectedTaskId, () => {
  const t = selectedTask.value;
  if (!t) return;
  outputTab.value = desiredOutputTabForTask(t);
  logShowAssistant.value = true;
  logShowStdout.value = true;
  logShowStderr.value = true;
  logShowSystem.value = true;
  logSearch.value = "";
  resumeExpanded.value = false;
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
  if (isPhone.value) sessionsDrawerOpen.value = false;
  runsOpen.value = false;
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

function dismissFeedCoach() {
  feedCoachDismissed.value = true;
  feedCoachOpen.value = false;
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
  if (!resumePrompt.value.trim()) return;
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

function isImeComposing(e: KeyboardEvent): boolean {
  // isComposing is reliable on modern browsers; keyCode=229 is a common fallback.
  return Boolean((e as any).isComposing) || (e as any).keyCode === 229;
}

async function onResumeEnter(e: KeyboardEvent) {
  if (isImeComposing(e)) return;
  e.preventDefault();
  await onResumeTask();
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

async function openWorkspaceFiles() {
  const sess = selectedSession.value;
  if (!sess) return;
  const base = (sess.workdir ?? "").trim() || ".";
  if (filesDirty.value && !window.confirm("Discard unsaved changes?")) return;

  filesOpen.value = true;
  filesLoading.value = true;
  filesError.value = "";
  filesNotice.value = "";
  filesBase.value = base;
  filesRoot.value = null;
  resetFilesEditor();

  try {
    const res = await fetchFSEntries(".", base);
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
  es.addEventListener("heartbeat", () => {
    eventsConnected.value = true;
    eventsLastHeartbeatMs.value = Date.now();
    eventsLastEventMs.value = eventsLastHeartbeatMs.value;
  });
}

function reconnectEvents() {
  connectEvents();
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
});

onBeforeUnmount(() => {
  if (es) es.close();
  window.removeEventListener("keydown", onGlobalKeyDown);
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
  const roots = workspaceFilters.value.map((x) => x.trim()).filter(Boolean);
  const needle = sessionSearch.value.trim().toLowerCase();
  return sessionsAll.value.filter((s) => {
    if (roots.length > 0 && !roots.some((r) => isWithinWorkspace(r, s.workdir)))
      return false;
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

const anyRunning = computed(() => secretaryCounts.value.running > 0);

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

function isMilestoneMessage(stream: LogEntry["stream"], message: string): boolean {
  const msg = (message ?? "").trim();
  if (!msg) return false;
  const lower = msg.toLowerCase();

  if (stream === "assistant") return true;

  if (stream === "system") {
    if (lower.startsWith("run.start")) return true;
    if (lower.startsWith("run.finish")) return true;
    if (lower.includes("blocked") || lower.includes("requires approval")) return true;
    if (lower.includes("error") || lower.includes("panic") || lower.includes("failed"))
      return true;
    if (lower.includes("skipped overlong") || lower.includes("read error"))
      return true;
    return false;
  }

  // stderr is noisy; keep only obvious problems.
  if (stream === "stderr") {
    if (lower.includes("error") || lower.includes("panic") || lower.includes("failed"))
      return true;
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

const liveItemsAll = computed<FeedItem[]>(() => {
  const scope = liveScope.value;
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

const liveItems = computed<FeedItem[]>(() => {
  if (liveMode.value === "all") return liveItemsAll.value;
  const out = liveItemsAll.value.filter((f) =>
    isMilestoneMessage(f.stream, f.message),
  );
  return out.map((f) => ({ ...f, message: summarizeForFeed(f.stream, f.message) }));
});

const liveLastTimeMsAll = computed(() => {
  const list = liveItemsAll.value;
  if (list.length === 0) return 0;
  return list[list.length - 1].time_ms;
});

const eventsIdleSeconds = computed(() => {
  const last = eventsLastEventMs.value;
  if (!last) return 0;
  const s = Math.floor((liveNowMs.value - last) / 1000);
  return s > 0 ? s : 0;
});

const feedIdleSeconds = computed(() => {
  // Idle should consider any output, not only milestones.
  const last = liveLastTimeMsAll.value;
  if (!last) return 0;
  const s = Math.floor((liveNowMs.value - last) / 1000);
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
    if (secretaryOpen.value) {
      closeSecretary();
      return;
    }
    if (liveOpen.value) {
      liveOpen.value = false;
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

watch(
  [liveOpen, liveScope],
  async ([open]) => {
    if (!open) return;
    await nextTick();
    // Backfill a small amount of logs to avoid "blank" Live after refresh.
    if (liveScope.value === "current") {
      const sess = selectedSession.value;
      if (sess) {
        const runs = sess.runs.slice(-6);
        await Promise.all(
          runs.map(async (r) => {
            const existing = logsByTask.value.get(r.id);
            if (existing && existing.length > 0) return;
            try {
              await loadLogs(r.id);
            } catch {
              // ignore
            }
          }),
        );
      }
    }
    if (!livePaused.value) {
      const el = liveBoxEl.value;
      if (el) el.scrollTop = el.scrollHeight;
    }
  },
  { immediate: false },
);

watch(
  () => liveItems.value.length,
  async () => {
    if (!liveOpen.value) return;
    if (livePaused.value) return;
    await nextTick();
    const el = liveBoxEl.value;
    if (el) el.scrollTop = el.scrollHeight;
  },
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
          v-if="isPhone"
          type="button"
          class="menuBtn"
          @click="sessionsDrawerOpen = true"
          title="Sessions"
          aria-label="Open sessions"
        >
          <span class="menuIcon" aria-hidden="true">≡</span>
        </button>
        <div class="title">ControlCCX</div>
      </div>
      <div class="headerRight">
        <div class="sub" v-if="systemInfo">
          {{ systemInfo.os }}/{{ systemInfo.arch }} ·
          {{ systemInfo.hostname }} · Go {{ systemInfo.go_version }}
        </div>
        <button type="button" class="themeBtn" @click="toggleTheme">
          {{ theme === "dark" ? "Day" : "Night" }}
        </button>
        <button
          type="button"
          class="liveBtn"
          @click="openLive"
          :title="anyRunning ? 'Open Live Feed (L · running)' : 'Open Live Feed (L)'"
        >
          <span v-if="anyRunning" class="liveDot" aria-hidden="true">●</span>
          Live
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

    <div v-if="isPhone && sessionsDrawerOpen" class="sessionsOverlay" @click.self="sessionsDrawerOpen = false"></div>

    <div class="grid">
      <section
        v-if="!isPhone || sessionsDrawerOpen"
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
            v-if="isPhone"
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
              @click="sessionsFiltersOpen = !sessionsFiltersOpen"
            >
              {{ sessionsFiltersOpen ? "Less" : "Filters" }}
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
                    <span class="mono">{{ p }}</span>
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

          <button
            v-for="s in pagedSessions"
            :key="s.key"
            class="row"
            :class="{ active: s.key === selectedSessionKey }"
            :title="s.latest.warning || s.warning || undefined"
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
              <span
                v-if="s.latest.warning || s.warning"
                class="warn"
                :title="s.latest.warning || s.warning"
                >⚠</span
              >
            </div>
            <div class="rowPath mono" :title="s.workdir">{{ s.workdir }}</div>
            <div class="rowBottom">{{ s.latest.prompt }}</div>
          </button>

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
        <div v-if="!selectedSession" class="empty">Select a session</div>
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
                <span class="pill" :class="selectedSession.status">{{
                  selectedSession.status
                }}</span>
                <span class="pill kind">{{ selectedSession.worker_type }}</span>
                <button
                  type="button"
                  class="detailMini detailMiniBtn"
                  @click="openRuns"
                  title="Open runs"
                >
                  {{ selectedSession.runs.length }} runs
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
                        @click="openWorkspaceFiles"
                        title="Browse workspace files"
                      >
                        Files
                      </button>
                      <button
                        type="button"
                        @click="copyText(selectedSession.workdir)"
                        title="Copy workdir"
                      >
                        Copy workdir
                      </button>
                    </div>
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
              Blocked: requires approval · 可尝试开启
              <span class="mono">workers.unsafe_automation</span> 后重试（危险）
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
                class="resumeToggle"
                @click="resumeExpanded = !resumeExpanded"
                :title="resumeExpanded ? 'Collapse' : 'Expand'"
              >
                {{ resumeExpanded ? "▴" : "⋯" }}
              </button>
            </div>
            <div v-if="!selectedSession.session_id" class="tinyHint">
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
                  <span class="logTime" :title="formatLocalDateTime(l.time)">{{
                    formatLogTime(l.time)
                  }}</span>
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
          </div>
          <button class="iconBtn" type="button" @click="closeSecretary">
            ✕
          </button>
        </div>

        <div class="secDrawerBody">
          <div v-if="secretaryView === 'overview'" class="secOverview">
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

	    <div v-if="liveOpen" class="secDrawerOverlay" @click.self="liveOpen = false">
	      <aside class="secDrawer wide" role="dialog" aria-modal="true">
        <div class="secDrawerHeader">
          <div class="secDrawerTitle">Live</div>
          <button class="iconBtn" type="button" @click="liveOpen = false">
            ✕
          </button>
        </div>
        <div class="secDrawerBody">
          <div class="secFeed">
            <div class="feedControls">
              <div class="feedLeft">
                <label class="feedLabel">
                  Scope
                  <select v-model="liveScope">
                    <option value="current">Current</option>
                    <option value="all">All</option>
                  </select>
                </label>
                <label class="feedLabel">
                  View
                  <select v-model="liveMode">
                    <option value="milestones">Milestones</option>
                    <option value="all">All Logs</option>
                  </select>
                </label>
                <label class="feedToggle">
                  <input type="checkbox" v-model="liveWrap" />
                  Wrap
                </label>
                <button type="button" @click="livePaused = !livePaused">
                  {{ livePaused ? "Resume" : "Pause" }}
                </button>
              </div>
              <div class="feedRight">
                <span
                  class="feedConn"
                  :class="{ bad: !eventsConnected || eventsIdleSeconds >= 25 }"
                  :title="
                    eventsConnected
                      ? `Connected · last event ${eventsIdleSeconds}s ago`
                      : `Disconnected · last event ${eventsIdleSeconds}s ago`
                  "
                >
                  {{ eventsConnected ? "Connected" : "Reconnecting…" }}
                </span>
                <button
                  v-if="!eventsConnected || eventsIdleSeconds >= 25"
                  type="button"
                  class="feedReconnect"
                  @click="reconnectEvents"
                  title="Reconnect event stream"
                >
                  Reconnect
                </button>
                <span
                  v-if="liveMode === 'milestones'"
                  class="feedHint"
                  title="Milestones show system run.start/run.finish, assistant output, and error-like lines."
                >
                  Milestones
                </span>
                <span
                  v-if="selectedTask?.status === 'running' && feedIdleSeconds >= 10"
                  class="feedIdle"
                  :title="
                    feedIdleSeconds >= 300
                      ? `Quiet for ${feedIdleSeconds}s · tools may be silent`
                      : `No log output for ${feedIdleSeconds}s`
                  "
                >
                  {{ feedIdleSeconds >= 300 ? "Quiet" : "No logs" }}
                  {{ feedIdleSeconds }}s
                </span>
              </div>
            </div>

            <div
              ref="liveBoxEl"
              class="feedBox"
              :class="{ wrap: liveWrap }"
              role="log"
              aria-label="Live feed"
            >
              <div v-if="liveItems.length === 0" class="empty">
                暂无日志（仅展示本次打开页面后收到的实时日志）
              </div>
              <div v-else class="feedLines">
                <div
                  v-for="(f, idx) in liveItems"
                  :key="f.task_id + ':' + f.time + ':' + idx"
                  class="feedLine"
                >
                  <span class="feedTime mono" :title="formatLocalDateTime(f.time)">{{
                    formatLogTime(f.time)
                  }}</span>
                  <span class="feedTask mono" :title="f.task_id">{{ f.task_short }}</span>
                  <span class="feedStream">{{ f.stream }}</span>
                  <span class="feedMsg">{{ f.message }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
	      </aside>
	    </div>

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
	                <span class="mono">{{ r.id.slice(0, 8) }}</span>
	                <span class="pill" :class="r.status">{{ r.status }}</span>
	              </div>
	              <div class="runMid">
	                <span class="pill kind">{{ r.mode }}</span>
	                <span class="score">score {{ r.score }}</span>
	                <span class="mono" :title="r.created_at">{{
	                  formatLocalDateTime(r.created_at)
	                }}</span>
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
              <span class="mono">L</span> live ·
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
      v-if="filesOpen"
      class="modalOverlay"
      @click.self="closeFiles"
    >
      <div class="modal filesModal">
        <div class="modalHeader">
          <div class="modalTitle">Files</div>
          <button class="iconBtn" type="button" @click="closeFiles">✕</button>
        </div>

        <div class="modalBody filesModalBody">
          <div v-if="filesError" class="modalError">
            {{ filesError }}
          </div>
          <div v-else-if="filesLoading" class="loading">Loading...</div>
          <template v-else>
            <div class="filesTopRow">
              <div class="mono filesRootPath" :title="filesRoot?.path">
                {{ filesRoot?.path }}
              </div>
              <div v-if="filesNotice" class="tinyHint">{{ filesNotice }}</div>
            </div>

            <div class="filesSplit">
              <div class="filesTreePane">
                <div class="filesTreeActions">
                  <button type="button" @click="filesNewFile" :disabled="!filesRoot">
                    New file
                  </button>
                  <button type="button" @click="filesNewFolder" :disabled="!filesRoot">
                    New folder
                  </button>
                  <button
                    type="button"
                    @click="filesDeleteSelected"
                    :disabled="
                      !filesSelectedPath || filesSelectedPath === filesRoot?.path
                    "
                  >
                    Delete
                  </button>
                  <button
                    type="button"
                    @click="filesRoot && refreshFilesDir(filesRoot.path)"
                    :disabled="!filesRoot"
                  >
                    Refresh
                  </button>
                </div>

                <div class="filesTreeList">
                  <button
                    v-for="v in filesVisibleNodes"
                    :key="v.node.path"
                    type="button"
                    class="filesNode"
                    :class="{
                      active:
                        normalizePathForCompare(v.node.path) ===
                        normalizePathForCompare(filesSelectedPath),
                    }"
                    :style="{ paddingLeft: `${12 + v.depth * 14}px` }"
                    @click="onFilesNodeClick(v.node)"
                  >
                    <span class="filesNodeTwisty">{{
                      v.node.kind === "dir" ? (v.node.expanded ? "▾" : "▸") : ""
                    }}</span>
                    <span class="filesNodeIcon">{{
                      v.node.kind === "dir" ? "📁" : "📄"
                    }}</span>
                    <span class="filesNodeName">{{ v.node.name }}</span>
                    <span
                      v-if="v.node.kind === 'file'"
                      class="filesNodeMeta mono"
                      >{{ v.node.size ?? 0 }}</span
                    >
                    <span v-if="v.node.loading" class="filesNodeMeta tinyHint"
                      >…</span
                    >
                  </button>
                  <div v-if="!filesVisibleNodes.length" class="empty">
                    Empty folder
                  </div>
                </div>
              </div>

              <div class="filesEditorPane">
                <div v-if="filesSelectedKind !== 'file'" class="empty">
                  {{
                    filesSelectedKind === "dir"
                      ? "Select a file to preview/edit."
                      : "Select a file."
                  }}
                </div>
                <template v-else>
                  <div class="filesEditorHeader">
                    <div class="mono filesEditorPath" :title="filesSelectedPath">
                      {{ filesSelectedPath }}
                    </div>
                    <span class="tinyHint mono">{{ filesFileSize }} bytes</span>
                    <span v-if="filesFileTruncated" class="pill warn"
                      >truncated</span
                    >
                    <div class="tabSpacer"></div>
                    <button
                      type="button"
                      @click="copyText(filesFileContent)"
                      :disabled="!filesFileContent"
                    >
                      Copy
                    </button>
                    <button
                      type="button"
                      class="primary"
                      @click="filesSave"
                      :disabled="filesSaving || !filesDirty || filesFileTruncated"
                    >
                      {{ filesSaving ? "Saving..." : "Save" }}
                    </button>
                  </div>

                  <div class="outputTabs">
                    <button
                      type="button"
                      class="tabBtn"
                      :class="{ active: filesView === 'preview' }"
                      @click="filesView = 'preview'"
                    >
                      Preview
                    </button>
                    <button
                      type="button"
                      class="tabBtn"
                      :class="{ active: filesView === 'edit' }"
                      @click="filesView = 'edit'"
                    >
                      Edit
                    </button>
                    <div class="tabSpacer"></div>
                    <div v-if="filesDirty" class="tinyHint">unsaved</div>
                  </div>

                  <div v-if="filesFileError" class="modalError">
                    {{ filesFileError }}
                  </div>
                  <div v-else-if="filesFileLoading" class="loading">Loading...</div>
                  <template v-else>
                    <div v-if="filesView === 'edit'" class="filesEditorEdit">
                      <textarea
                        v-model="filesFileContent"
                        rows="18"
                        spellcheck="false"
                      ></textarea>
                    </div>
                    <template v-else>
                      <div
                        v-if="filesIsMarkdown"
                        class="resultBox markdown filePreviewBox"
                        v-html="filesPreviewHtml"
                        @click="onFilesPreviewMarkdownClick"
                      ></div>

                      <div v-else class="resultBox fileCodeBox">
                        <pre class="hljs"><code v-html="filesCodeHtml"></code></pre>
                      </div>
                    </template>
                  </template>
                </template>
              </div>
            </div>
          </template>
        </div>

        <div class="modalFooter">
          <button type="button" @click="closeFiles">Close</button>
        </div>
      </div>
    </div>

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
  top: calc(90px + env(safe-area-inset-top));
  right: 16px;
  bottom: max(16px, env(safe-area-inset-bottom));
  width: min(440px, calc(100vw - 32px));
  background: var(--bg-panel);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--border-color);
  overflow: hidden;
  display: grid;
  grid-template-rows: auto 1fr;
}

.secDrawer.wide {
  width: min(560px, calc(100vw - 32px));
}

.feedCoach {
  position: fixed;
  right: 88px;
  bottom: 28px;
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
  grid-template-columns: 340px 1fr;
  gap: 12px;
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

.modalError {
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
  overflow: hidden;
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

.pinnedBtn .mono {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  margin-bottom: 6px;
}

.rowMid {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-sub);
  font-size: 11px;
  margin-bottom: 6px;
}

.rowPath {
  font-size: 11px;
  color: var(--text-sub);
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: rgba(15, 23, 42, 0.06);
  padding: 2px 8px;
  border-radius: 999px;
}

.rowBottom {
  font-size: 12px;
  color: var(--text-main);
  display: -webkit-box;
  line-clamp: 1;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.35;
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
  grid-template-columns: 1fr auto auto;
  gap: 10px;
  align-items: start;
}

.resumeRow textarea {
  min-height: 0;
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
  width: 100%;
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

@media (max-width: 900px) {
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

  .secTab,
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
  .header {
    padding: 12px 16px;
  }

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

  .secDrawer.wide {
    width: 100vw;
  }

  .feedCoach {
    right: 16px;
    left: 16px;
    bottom: 84px;
    max-width: none;
  }
}
</style>

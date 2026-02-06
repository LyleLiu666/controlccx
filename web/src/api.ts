import type {
  AcceptanceResponse,
  AuthInfo,
  AuthPatch,
  AuthStatus,
  ChatMessage,
  ProviderActivateResponse,
  ProviderDeleteResponse,
  ProviderProfile,
  ProvidersListResponse,
  ProviderSpeedTestResponse,
  ProviderUpsertResponse,
  FSDeleteResponse,
  FSEntriesResponse,
  FSListResponse,
  FSReadResponse,
  FSRoot,
  FSMkdirResponse,
  FSWriteResponse,
  LogEntry,
  ToolsListResponse,
  ToolsStatusResponse,
  SkillsListResponse,
  SkillsToolsResponse,
  OnboardingPlan,
  ManagedSkill,
  ProjectContext,
  GitCandidatesResponse,
  InstallGitBatchInput,
  InstallGitBatchResponse,
  PerSkillVersionsListResponse,
  PromptTemplate,
  PromptTemplateKind,
  RestoreSkillVersionResult,
  SkillVersion,
  SkillVersionsListResponse,
  ControlPlaneStatus,
  SystemInfo,
  SessionWorkspaceGetResponse,
  SessionWorkspaceMergeResponse,
  Task,
  TaskTraceResponse,
  Tool,
  WorkerType,
} from "./types";

export class APIError extends Error {
  status: number;
  statusText: string;
  data: any;
  rawText: string;

  constructor(opts: { status: number; statusText: string; message: string; data: any; rawText: string }) {
    super(opts.message);
    this.name = "APIError";
    this.status = opts.status;
    this.statusText = opts.statusText;
    this.data = opts.data;
    this.rawText = opts.rawText;
  }
}

export function isAPIError(e: unknown): e is APIError {
  return e instanceof APIError;
}

async function parseErrorPayload(res: Response): Promise<{ data: any; rawText: string }> {
  const rawText = await res.text();
  const ct = String(res.headers.get("Content-Type") || "").toLowerCase();
  if (ct.includes("application/json")) {
    try {
      return { data: JSON.parse(rawText), rawText };
    } catch {
      // fall back to raw text
    }
  }
  return { data: rawText, rawText };
}

async function buildAPIError(res: Response): Promise<APIError> {
  const { data, rawText } = await parseErrorPayload(res);
  let msg =
    typeof data?.message === "string" && data.message.trim()
      ? String(data.message)
      : rawText.trim()
        ? `${res.status} ${rawText.trim()}`
        : `${res.status} ${res.statusText}`;
  const hint = typeof data?.hint === "string" ? data.hint.trim() : "";
  if (hint && !msg.includes(hint)) msg = `${msg} (hint: ${hint})`;
  return new APIError({
    status: res.status,
    statusText: res.statusText,
    message: msg,
    data,
    rawText,
  });
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin" });
  if (!res.ok) throw await buildAPIError(res);
  return (await res.json()) as T;
}

async function postJSON<T>(
  path: string,
  body: unknown,
  opts?: { headers?: Record<string, string> },
): Promise<T> {
  const extraHeaders = opts?.headers ?? {};
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...extraHeaders },
    body: JSON.stringify(body),
    credentials: "same-origin",
  });
  if (!res.ok) throw await buildAPIError(res);
  return (await res.json()) as T;
}

export async function fetchSystemInfo(): Promise<SystemInfo> {
  return getJSON<SystemInfo>("/api/system");
}

export async function fetchControlPlaneStatus(): Promise<ControlPlaneStatus> {
  return getJSON<ControlPlaneStatus>("/api/control-plane");
}

export async function fetchToolsStatus(): Promise<ToolsStatusResponse> {
  return getJSON<ToolsStatusResponse>("/api/tools/status");
}

export async function fetchTasks(limit = 200, includeDeleted = false): Promise<Task[]> {
  const qs = new URLSearchParams({ limit: String(limit) });
  if (includeDeleted) qs.set("include_deleted", "1");
  const res = await getJSON<{ tasks: Task[] }>(`/api/tasks?${qs.toString()}`);
  return res.tasks;
}

export async function fetchAcceptance(key: string): Promise<AcceptanceResponse> {
  const qs = new URLSearchParams({ key: String(key ?? "") });
  return getJSON<AcceptanceResponse>(`/api/acceptance?${qs.toString()}`);
}

export async function fetchProjectContext(): Promise<ProjectContext> {
  return getJSON<ProjectContext>("/api/context");
}

export async function setProjectContext(content: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/context", { content: String(content ?? "") });
}

export async function fetchPromptTemplates(kind: PromptTemplateKind | "all" = "all"): Promise<PromptTemplate[]> {
  const qs = new URLSearchParams();
  const k = String(kind ?? "").trim();
  if (k) qs.set("kind", k);
  const res = await getJSON<{ templates: PromptTemplate[] }>(`/api/templates?${qs.toString()}`);
  return res.templates ?? [];
}

export async function upsertPromptTemplate(input: {
  id?: string;
  title: string;
  kind: PromptTemplateKind;
  content: string;
}): Promise<PromptTemplate> {
  const res = await postJSON<{ template: PromptTemplate }>("/api/templates/upsert", input);
  return res.template;
}

export async function deletePromptTemplate(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/templates/delete", { id: String(id ?? "") });
}

export async function createTask(input: {
  worker_type: WorkerType;
  prompt: string;
  workdir: string;
  conversation_id?: string;
  workdir_strategy?: string;
  worktree_untracked?: string;
  unsafe_automation?: boolean;
  safety_envelope?: string;
  safety_preset?: string;
  task_intent?: string;
  codex_sandbox?: string;
  codex_approval_policy?: string;
  codex_search?: boolean;
  claude_permission_mode?: string;
  claude_sandbox?: boolean;
  claude_webfetch_domains?: string[];
}, opts?: { idempotencyKey?: string }): Promise<Task> {
  const idempotencyKey = String(opts?.idempotencyKey ?? "").trim();
  const headers: Record<string, string> = {};
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;
  return postJSON<Task>("/api/tasks", { ...input, mode: "new" }, { headers });
}

export async function cancelTask(id: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/tasks/${id}/cancel`, {});
}

export async function resumeTask(id: string, prompt: string): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/resume`, { prompt });
}

export async function resumeTaskWithOptions(
  id: string,
  input: {
    prompt: string;
    unsafe_automation?: boolean;
    safety_envelope?: string;
    safety_preset?: string;
    task_intent?: string;
    codex_sandbox?: string;
    codex_approval_policy?: string;
    codex_search?: boolean;
    claude_permission_mode?: string;
    claude_sandbox?: boolean;
    claude_webfetch_domains?: string[];
  },
): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/resume`, input);
}

export async function continueSessionWithOptions(
  key: string,
  input: {
    prompt: string;
    unsafe_automation?: boolean;
    safety_envelope?: string;
    safety_preset?: string;
    task_intent?: string;
    codex_sandbox?: string;
    codex_approval_policy?: string;
    codex_search?: boolean;
    claude_permission_mode?: string;
    claude_sandbox?: boolean;
    claude_webfetch_domains?: string[];
  },
): Promise<Task> {
  return postJSON<Task>(`/api/sessions/${encodeURIComponent(key)}/continue`, input);
}

export async function rehydrateTaskWithOptions(
  id: string,
  input: {
    prompt?: string;
    unsafe_automation?: boolean;
    safety_envelope?: string;
    safety_preset?: string;
    task_intent?: string;
    codex_sandbox?: string;
    codex_approval_policy?: string;
    codex_search?: boolean;
    claude_permission_mode?: string;
    claude_sandbox?: boolean;
    claude_webfetch_domains?: string[];
  } = {},
): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/rehydrate`, input);
}

export async function renameSession(key: string, title: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/sessions/${encodeURIComponent(key)}/rename`, { title });
}

export async function deleteSession(key: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/sessions/${encodeURIComponent(key)}/delete`, {});
}

export async function fetchSessionWorkspace(key: string): Promise<SessionWorkspaceGetResponse> {
  return getJSON<SessionWorkspaceGetResponse>(`/api/sessions/${encodeURIComponent(key)}/workspace`);
}

export async function mergeSessionWorkspace(key: string): Promise<SessionWorkspaceMergeResponse> {
  return postJSON<SessionWorkspaceMergeResponse>(`/api/sessions/${encodeURIComponent(key)}/workspace/merge`, {});
}

export async function discardSessionWorkspace(key: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/api/sessions/${encodeURIComponent(key)}/workspace/discard`, {});
}

export async function fetchLogs(taskId: string, after = 0, limit = 500): Promise<LogEntry[]> {
  const res = await getJSON<{ logs: LogEntry[] }>(
    `/api/tasks/${taskId}/logs?after=${after}&limit=${limit}`
  );
  return res.logs;
}

export async function fetchTaskTrace(taskId: string): Promise<TaskTraceResponse> {
  return getJSON<TaskTraceResponse>(`/api/tasks/${taskId}/trace`);
}

export async function fetchChat(after = 0, limit = 200): Promise<ChatMessage[]> {
  const res = await getJSON<{ messages: ChatMessage[] }>(`/api/chat?after=${after}&limit=${limit}`);
  return res.messages;
}

export async function sendChat(message: string): Promise<{ message: string }> {
  return postJSON<{ message: string }>("/api/chat", { message });
}

export type ChatSendOptions = {
  backend?: "auto" | "simple-http" | "claude" | "codex";
  max_steps?: number;
};

export type ChatStreamEvent = {
  event: string;
  data: any;
};

export async function sendChatStream(
  message: string,
  opts: ChatSendOptions,
  onEvent: (evt: ChatStreamEvent) => void,
): Promise<string> {
  const res = await fetch("/api/chat?stream=1", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify({ message, stream: true, ...opts }),
    credentials: "same-origin",
  });

  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);

  const ct = (res.headers.get("Content-Type") || "").toLowerCase();
  if (!ct.includes("text/event-stream")) {
    const data = (await res.json()) as { message?: string };
    const msg = data?.message ?? "";
    onEvent({ event: "final", data: { message: msg } });
    return msg;
  }

  if (!res.body) throw new Error("missing response body");

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finalMessage = "";

  const flush = (chunk: string) => {
    buffer += chunk.replaceAll("\r", "");
    while (true) {
      const idx = buffer.indexOf("\n\n");
      if (idx < 0) break;
      const block = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      const lines = block.split("\n").filter(Boolean);
      let event = "message";
      const dataLines: string[] = [];
      for (const line of lines) {
        if (line.startsWith("event:")) {
          event = line.slice("event:".length).trim();
          continue;
        }
        if (line.startsWith("data:")) {
          dataLines.push(line.slice("data:".length).trim());
        }
      }
      const dataRaw = dataLines.join("\n");
      let data: any = dataRaw;
      try {
        data = JSON.parse(dataRaw);
      } catch {
        // ignore
      }
      onEvent({ event, data });
      if (event === "final" && data && typeof data.message === "string") {
        finalMessage = data.message;
      }
      if (event === "error" && data && typeof data.error === "string") {
        throw new Error(data.error);
      }
    }
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    if (value) flush(decoder.decode(value, { stream: true }));
  }
  flush(decoder.decode());
  return finalMessage;
}

export async function fetchFSRoots(): Promise<FSRoot[]> {
  const res = await getJSON<{ roots: FSRoot[] }>("/api/fs/roots");
  return res.roots;
}

export async function fetchFSList(path: string): Promise<FSListResponse> {
  const url = `/api/fs/list?path=${encodeURIComponent(path)}`;
  return getJSON<FSListResponse>(url);
}

export async function fetchFSRead(path: string, base?: string): Promise<FSReadResponse> {
  const qs = new URLSearchParams({ path });
  if (base && base.trim()) qs.set("base", base);
  return getJSON<FSReadResponse>(`/api/fs/read?${qs.toString()}`);
}

export async function fetchFSEntries(path: string, base?: string): Promise<FSEntriesResponse> {
  const qs = new URLSearchParams({ path });
  if (base && base.trim()) qs.set("base", base);
  return getJSON<FSEntriesResponse>(`/api/fs/entries?${qs.toString()}`);
}

export async function fsWrite(input: {
  path: string;
  base?: string;
  content: string;
}): Promise<FSWriteResponse> {
  return postJSON<FSWriteResponse>("/api/fs/write", input);
}

export async function fsMkdir(input: {
  path: string;
  base?: string;
  recursive?: boolean;
}): Promise<FSMkdirResponse> {
  return postJSON<FSMkdirResponse>("/api/fs/mkdir", input);
}

export async function fsDelete(input: {
  path: string;
  base?: string;
  recursive?: boolean;
}): Promise<FSDeleteResponse> {
  return postJSON<FSDeleteResponse>("/api/fs/delete", input);
}

export async function fetchAuthStatus(): Promise<AuthStatus> {
  return getJSON<AuthStatus>("/api/auth/status");
}

export async function fetchAuthInfo(): Promise<AuthInfo> {
  return getJSON<AuthInfo>("/api/auth");
}

export async function updateAuth(patch: AuthPatch): Promise<AuthInfo> {
  return postJSON<AuthInfo>("/api/auth", patch);
}

export async function fetchProviders(): Promise<ProvidersListResponse> {
  return getJSON<ProvidersListResponse>("/api/providers");
}

export async function upsertProvider(profile: ProviderProfile): Promise<ProviderUpsertResponse> {
  return postJSON<ProviderUpsertResponse>("/api/providers/upsert", { profile });
}

export async function deleteProvider(id: string): Promise<ProviderDeleteResponse> {
  return postJSON<ProviderDeleteResponse>("/api/providers/delete", { id });
}

export async function activateProvider(input: {
  target: "claude" | "codex" | "secretary";
  id: string;
  force_live_overwrite?: boolean;
  claude_home_dir?: string;
  codex_home_dir?: string;
}): Promise<ProviderActivateResponse> {
  return postJSON<ProviderActivateResponse>("/api/providers/activate", input);
}

export async function speedtestProvider(input: {
  target: "claude" | "codex";
  id: string;
  timeout_ms?: number;
}): Promise<ProviderSpeedTestResponse> {
  return postJSON<ProviderSpeedTestResponse>("/api/providers/speedtest", input);
}

export async function fetchTools(): Promise<ToolsListResponse> {
  return getJSON<ToolsListResponse>("/api/tools");
}

export async function upsertTool(input: { tool: Tool }): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/tools/upsert", input);
}

export async function deleteTool(input: { id: string }): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/tools/delete", input);
}

export async function fetchSkills(opts?: {
  q?: string;
  offset?: number;
  limit?: number;
}): Promise<SkillsListResponse> {
  const qs = new URLSearchParams();
  const q = (opts?.q ?? "").trim();
  if (q) qs.set("q", q);
  if (Number.isFinite(opts?.offset)) qs.set("offset", String(opts?.offset));
  if (Number.isFinite(opts?.limit)) qs.set("limit", String(opts?.limit));
  const url = qs.toString() ? `/api/skills?${qs.toString()}` : "/api/skills";
  return getJSON<SkillsListResponse>(url);
}

export async function linkSkill(input: {
  name: string;
  target: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
  auto_import?: boolean;
  prefer_tool?: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
}): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/skills/link", input);
}

export async function unlinkSkill(input: {
  name: string;
  target: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
}): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/skills/unlink", input);
}

export async function syncSkill(input: {
  name: string;
  target: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
  overwrite?: boolean;
}): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/skills/sync", input);
}

export async function fetchSkillsTools(): Promise<SkillsToolsResponse> {
  return getJSON<SkillsToolsResponse>("/api/skills/tools");
}

export async function fetchSkillsOnboarding(): Promise<OnboardingPlan> {
  return getJSON<OnboardingPlan>("/api/skills/onboarding");
}

export async function importExistingSkill(input: {
  source_path: string;
  name: string;
  tool?: string;
  overwrite?: boolean;
}): Promise<ManagedSkill> {
  return postJSON<ManagedSkill>("/api/skills/import", input);
}

export async function installSkillLocal(input: {
  source_path: string;
  name?: string;
  overwrite?: boolean;
}): Promise<ManagedSkill> {
  return postJSON<ManagedSkill>("/api/skills/install/local", input);
}

export async function listGitSkillCandidates(input: {
  repo_url: string;
}): Promise<GitCandidatesResponse> {
  return postJSON<GitCandidatesResponse>("/api/skills/git/candidates", input);
}

export async function installSkillGit(input: {
  repo_url: string;
  subpath?: string;
  name?: string;
  overwrite?: boolean;
}): Promise<ManagedSkill> {
  return postJSON<ManagedSkill>("/api/skills/install/git", input);
}

export async function installSkillGitBatch(input: InstallGitBatchInput): Promise<InstallGitBatchResponse> {
  return postJSON<InstallGitBatchResponse>("/api/skills/install/git/batch", input);
}

export async function updateManagedSkill(input: { name: string }): Promise<ManagedSkill> {
  return postJSON<ManagedSkill>("/api/skills/update", input);
}

export async function fetchSkillVersions(): Promise<SkillVersionsListResponse> {
  return getJSON<SkillVersionsListResponse>("/api/skills/versions");
}

export async function createSkillVersion(input: {
  id?: string;
  note?: string;
}): Promise<SkillVersion> {
  return postJSON<SkillVersion>("/api/skills/versions/create", input);
}

export async function deleteSkillVersion(input: { id: string }): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>("/api/skills/versions/delete", input);
}

export async function fetchSkillVersionsBySkill(name: string): Promise<PerSkillVersionsListResponse> {
  const n = String(name ?? "").trim();
  return getJSON<PerSkillVersionsListResponse>(`/api/skills/${encodeURIComponent(n)}/versions`);
}

export async function createSkillVersionBySkill(
  name: string,
  input: {
    id?: string;
    note?: string;
  },
): Promise<SkillVersion> {
  const n = String(name ?? "").trim();
  return postJSON<SkillVersion>(`/api/skills/${encodeURIComponent(n)}/versions/create`, input);
}

export async function deleteSkillVersionBySkill(
  name: string,
  input: { id: string },
): Promise<{ ok: boolean }> {
  const n = String(name ?? "").trim();
  return postJSON<{ ok: boolean }>(`/api/skills/${encodeURIComponent(n)}/versions/delete`, input);
}

export async function restoreSkillVersionBySkill(
  name: string,
  input: { id: string; backup?: boolean; backup_note?: string },
): Promise<RestoreSkillVersionResult> {
  const n = String(name ?? "").trim();
  return postJSON<RestoreSkillVersionResult>(`/api/skills/${encodeURIComponent(n)}/versions/restore`, input);
}

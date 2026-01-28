import type {
  AuthInfo,
  AuthPatch,
  AuthStatus,
  ChatMessage,
  FSDeleteResponse,
  FSEntriesResponse,
  FSListResponse,
  FSReadResponse,
  FSRoot,
  FSMkdirResponse,
  FSWriteResponse,
  LogEntry,
  SystemInfo,
  Task,
  WorkerType,
} from "./types";

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin" });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return (await res.json()) as T;
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    credentials: "same-origin",
  });
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return (await res.json()) as T;
}

export async function fetchSystemInfo(): Promise<SystemInfo> {
  return getJSON<SystemInfo>("/api/system");
}

export async function fetchTasks(limit = 200, includeDeleted = false): Promise<Task[]> {
  const qs = new URLSearchParams({ limit: String(limit) });
  if (includeDeleted) qs.set("include_deleted", "1");
  const res = await getJSON<{ tasks: Task[] }>(`/api/tasks?${qs.toString()}`);
  return res.tasks;
}

export async function createTask(input: {
  worker_type: WorkerType;
  prompt: string;
  workdir: string;
  unsafe_automation?: boolean;
}): Promise<Task> {
  return postJSON<Task>("/api/tasks", { ...input, mode: "new" });
}

export async function cancelTask(id: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/tasks/${id}/cancel`, {});
}

export async function resumeTask(id: string, prompt: string): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/resume`, { prompt });
}

export async function resumeTaskWithOptions(
  id: string,
  input: { prompt: string; unsafe_automation?: boolean },
): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/resume`, input);
}

export async function renameSession(key: string, title: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/sessions/${encodeURIComponent(key)}/rename`, { title });
}

export async function deleteSession(key: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/sessions/${encodeURIComponent(key)}/delete`, {});
}

export async function fetchLogs(taskId: string, after = 0, limit = 500): Promise<LogEntry[]> {
  const res = await getJSON<{ logs: LogEntry[] }>(
    `/api/tasks/${taskId}/logs?after=${after}&limit=${limit}`
  );
  return res.logs;
}

export async function fetchChat(after = 0, limit = 200): Promise<ChatMessage[]> {
  const res = await getJSON<{ messages: ChatMessage[] }>(`/api/chat?after=${after}&limit=${limit}`);
  return res.messages;
}

export async function sendChat(message: string): Promise<{ message: string }> {
  return postJSON<{ message: string }>("/api/chat", { message });
}

export type ChatSendOptions = {
  backend?: "auto" | "claude" | "codex";
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

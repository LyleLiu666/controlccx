import type { ChatMessage, LogEntry, SystemInfo, Task, WorkerType } from "./types";

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

export async function fetchTasks(limit = 200): Promise<Task[]> {
  const res = await getJSON<{ tasks: Task[] }>(`/api/tasks?limit=${limit}`);
  return res.tasks;
}

export async function createTask(input: {
  worker_type: WorkerType;
  prompt: string;
  workdir: string;
}): Promise<Task> {
  return postJSON<Task>("/api/tasks", { ...input, mode: "new" });
}

export async function cancelTask(id: string): Promise<{ ok: boolean }> {
  return postJSON(`/api/tasks/${id}/cancel`, {});
}

export async function resumeTask(id: string, prompt: string): Promise<Task> {
  return postJSON<Task>(`/api/tasks/${id}/resume`, { prompt });
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


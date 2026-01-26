export type TaskStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "canceled"
  | "interrupted"
  | "blocked";

export type WorkerType = "claude-code" | "codex" | "exec";

export type Task = {
  id: string;
  worker_type: WorkerType;
  mode: "new" | "resume";
  status: TaskStatus;
  prompt: string;
  workdir: string;
  session_id: string;
  warning: string;
  error: string;
  exit_code?: number;
  stderr_count: number;
  keyword_count: number;
  score: number;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
};

export type LogEntry = {
  id: number;
  task_id: string;
  time: string;
  stream: "stdout" | "stderr" | "system" | "assistant";
  message: string;
};

export type ChatMessage = {
  id: number;
  time: string;
  role: "user" | "assistant";
  content: string;
};

export type SystemInfo = {
  hostname: string;
  os: string;
  arch: string;
  go_version: string;
  now: string;
};

export type ServerEvent = {
  type: string;
  time: string;
  payload?: any;
};


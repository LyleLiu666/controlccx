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

export type FSRoot = {
  name: string;
  path: string;
};

export type FSListEntry = {
  name: string;
  path: string;
};

export type FSListResponse = {
  path: string;
  parent?: string;
  entries: FSListEntry[];
};

export type ServerEvent = {
  type: string;
  time: string;
  payload?: any;
};

export type AuthFieldStatus = {
  effective: "env" | "stored" | "codex" | "default" | "none";
  masked?: string;
};

export type AuthStatus = {
  claude: {
    base_url: AuthFieldStatus;
    api_key: AuthFieldStatus;
    auth_token: AuthFieldStatus;
    model: AuthFieldStatus;
    small_fast_model: AuthFieldStatus;
    available: boolean;
  };
  codex: {
    api_key: AuthFieldStatus;
    model: AuthFieldStatus;
    reasoning_effort: AuthFieldStatus;
    available: boolean;
  };
};

export type AuthInfo = {
  status: AuthStatus;
  storage_path: string;
};

export type AuthPatch = {
  anthropic_base_url?: string;
  anthropic_api_key?: string;
  anthropic_auth_token?: string;
  anthropic_model?: string;
  anthropic_small_fast_model?: string;
  openai_api_key?: string;
  codex_model?: string;
  codex_reasoning_effort?: string;
};

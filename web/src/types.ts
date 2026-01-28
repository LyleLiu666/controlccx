export type TaskStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "canceled"
  | "interrupted"
  | "blocked";

export type WorkerType = "claude-code" | "codex";

export type Task = {
  id: string;
  worker_type: WorkerType;
  mode: "new" | "resume";
  status: TaskStatus;
  unsafe_automation?: boolean;
  prompt: string;
  workdir: string;
  session_id: string;
  session_title?: string;
  session_deleted_at?: string;
  warning: string;
  error: string;
  exit_code?: number;
  finish_reason?: string;
  suggested_tests?: string[];
  stderr_count: number;
  keyword_count: number;
  score: number;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
};

export type SkillTargetRoot = {
  target: "claude" | "codex";
  root: string;
};

export type SkillTargetState = {
  target: "claude" | "codex";
  root: string;
  status:
    | "missing"
    | "linked"
    | "broken"
    | "present"
    | "copied"
    | "conflict"
    | "external";
  link_target?: string;
  note?: string;
};

export type Skill = {
  name: string;
  sources?: string[];
  source?: string;
  targets?: SkillTargetState[];
};

export type SkillsListResponse = {
  source_roots: string[];
  targets: SkillTargetRoot[];
  skills: Skill[];
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

export type FSReadResponse = {
  path: string;
  size: number;
  truncated: boolean;
  content: string;
};

export type FSEntryKind = "dir" | "file";

export type FSEntry = {
  name: string;
  path: string;
  kind: FSEntryKind;
  size?: number;
};

export type FSEntriesResponse = {
  path: string;
  parent?: string;
  entries: FSEntry[];
};

export type FSWriteResponse = {
  ok: boolean;
  path: string;
  bytes: number;
};

export type FSMkdirResponse = {
  ok: boolean;
  path: string;
};

export type FSDeleteResponse = {
  ok: boolean;
  path: string;
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

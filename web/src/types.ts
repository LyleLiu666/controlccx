export type TaskStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "canceled"
  | "interrupted"
  | "blocked";

export type WorkerType = string;

export type ToolDriver = "claude-code" | "codex" | "exec";

export type Tool = {
  id: string;
  driver: ToolDriver;
  command: string;
  args?: string[];
  env?: Record<string, string>;
};

export type ToolsListResponse = {
  tools: Tool[];
};

export type Task = {
  id: string;
  conversation_id: string;
  worker_type: WorkerType;
  mode: "new" | "resume";
  status: TaskStatus;
  unsafe_automation?: boolean;
  safety_preset?: string;
  task_intent?: string;
  codex_sandbox?: string;
  codex_approval_policy?: string;
  codex_search?: boolean;
  claude_permission_mode?: string;
  claude_sandbox?: boolean;
  claude_webfetch_domains?: string[];
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

export type AcceptanceState = {
  key: string;
  status: string;
  iteration: number;
  max_iterations: number;
  current_gate: string;
  summary: string;
  plan_json?: string;
  report?: string;
  run_id?: string;
  updated_at: string;
};

export type AcceptanceResponse = {
  ok: boolean;
  state: AcceptanceState | null;
};

export type SkillTargetRoot = {
  target: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
  root: string;
};

export type SkillTargetState = {
  target: "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
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
  total?: number;
  offset?: number;
  limit?: number;
};

export type SkillsToolInfo = {
  key: string;
  display_name: string;
  installed: boolean;
  detect_paths?: string[];
  skills_roots?: string[];
};

export type SkillsToolsResponse = {
  tools: SkillsToolInfo[];
};

export type OnboardingVariant = {
  tool: string;
  root: string;
  name: string;
  path: string;
  fingerprint?: string;
  is_link?: boolean;
  link_target?: string;
};

export type OnboardingGroup = {
  name: string;
  variants: OnboardingVariant[];
  has_conflict: boolean;
};

export type OnboardingPlan = {
  total_tools_scanned: number;
  total_skills_found: number;
  groups: OnboardingGroup[];
};

export type ManagedSkill = {
  name: string;
  path: string;
  source_type?: string;
  source_tool?: string;
  source_ref?: string;
  source_branch?: string;
  source_subpath?: string;
  source_revision?: string;
  content_hash?: string;
  created_at?: string;
  updated_at?: string;
};

export type GitSkillCandidate = {
  name: string;
  description?: string;
  subpath: string;
};

export type GitCandidatesResponse = {
  candidates: GitSkillCandidate[];
};

export type InstallGitBatchItem = {
  subpath: string;
  name?: string;
};

export type InstallGitBatchInput = {
  repo_url: string;
  skills: InstallGitBatchItem[];
  targets?: Array<"cursor" | "claude_code" | "codex" | "antigravity" | "opencode">;
  overwrite?: boolean;
};

export type InstallGitBatchResponse = {
  installed: ManagedSkill[];
};

export type SkillVersion = {
  id: string;
  created_at?: string;
  note?: string;
};

export type SkillVersionsListResponse = {
  source_root: string;
  versions_root: string;
  versions: SkillVersion[];
};

export type PerSkillVersionsListResponse = {
  skill: string;
  source_root: string;
  skill_source: string;
  versions_root: string;
  versions: SkillVersion[];
};

export type LogEntry = {
  id: number;
  task_id: string;
  time: string;
  stream: "stdout" | "stderr" | "system" | "assistant";
  message: string;
};

export type TaskInvocation = {
  task_id: string;
  cmd: string;
  args: string[];
  dir: string;
  env_injected_keys: string[];
  created_at: string;
};

export type TaskTraceResponse = {
  task: Task;
  invocation?: TaskInvocation | null;
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

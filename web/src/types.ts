export type TaskStatus =
  | "queued"
  | "waiting"
  | "running"
  | "awaiting_approval"
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

export type ToolStatus = {
  id: string;
  driver: ToolDriver;
  command: string;
  available: boolean;
  resolved_path?: string;
  error?: string;
};

export type ToolsStatusResponse = {
  tools: ToolStatus[];
};

export type Task = {
  id: string;
  conversation_id: string;
  worker_type: WorkerType;
  mode: "new" | "resume";
  status: TaskStatus;
  unsafe_automation?: boolean;
  workdir_strategy?: string;
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
  base_workdir?: string;
  worktree_dir?: string;
  worktree_branch?: string;
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

export type SessionContinueQueueState = "pending" | "dispatching" | "done" | "canceled" | "failed";

export type SessionContinueQueueItem = {
  id: string;
  conversation_id: string;
  prompt: string;
  run_options_json: string;
  priority: number;
  state: SessionContinueQueueState;
  source: string;
  created_at: string;
  updated_at: string;
};

export type QueueAck = {
  queued: true;
  queue_id: string;
  position: number;
  existing_task_id?: string;
  existing_status?: TaskStatus;
  preempted_task_id?: string;
};

export type TaskMutationAction =
  | "task.create"
  | "task.resume"
  | "task.rehydrate"
  | "task.enter_unsafe"
  | "session.continue"
  | "session.preempt_continue";

export type TaskMutationSuccess = {
  ok: true;
  action: TaskMutationAction;
  task?: Task;
  queue?: QueueAck;
  meta?: Record<string, any>;
};

export type TaskMutationProblem = {
  ok: false;
  error:
    | "invalid_argument"
    | "not_found"
    | "workdir_busy"
    | "session_task_in_flight"
    | "runner_unavailable"
    | "unsupported"
    | "internal";
  message: string;
  hint?: string;
  details?: Record<string, any>;
};

export type ContinueResponse = TaskMutationSuccess;

export type ApprovalStatus = "pending" | "approved" | "denied" | "expired";

export type RiskLevel = "low" | "medium" | "high";

export type ApprovalRequest = {
  id: string;
  task_id: string;
  worker_type: WorkerType;
  workdir: string;
  action_type: string;
  risk_level: RiskLevel;
  summary: string;
  raw: any;
  status: ApprovalStatus;
  reason: string;
  created_at: string;
  updated_at: string;
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

export type ProjectContext = {
  content: string;
  updated_at?: string;
};

export type PromptTemplateKind = "task" | "chat";

export type PromptTemplate = {
  id: string;
  title: string;
  kind: PromptTemplateKind;
  content: string;
  created_at?: string;
  updated_at?: string;
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
  versions_count?: number;
  latest_version_id?: string;
  new_version?: boolean;
  new_version_at?: string;
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

export type ManagedSkillManifest = {
  schema_version?: number;
  name: string;
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
  manifest?: ManagedSkillManifest;
  versions: SkillVersion[];
};

export type RestoreSkillVersionResult = {
  ok: boolean;
  skill?: string;
  id?: string;
  backup_id?: string;
  path?: string;
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

export type SessionWorkspace = {
  key: string;
  kind: string;
  base_workdir: string;
  repo_root: string;
  run_root: string;
  run_workdir: string;
  base_branch: string;
  work_branch: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type SessionWorkspaceGetResponse = {
  ok: boolean;
  workspace?: SessionWorkspace | null;
};

export type SessionWorkspaceMergeResponse = {
  workspace: SessionWorkspace;
  applied?: string[];
  conflicts?: string[];
};

export type SessionWorkspaceEnsureResponse = {
  ok: boolean;
  workspace?: SessionWorkspace | null;
  logs?: string[];
};

export type SystemInfo = {
  hostname: string;
  os: string;
  arch: string;
  go_version: string;
  now: string;
};

export type ControlPlaneComponent = {
  ok: boolean;
  name: string;
  protocol_version?: number;
  pid?: number;
  addr?: string;
  error?: string;
  ts_ms?: number;
};

export type ControlPlaneStatus = {
  server: ControlPlaneComponent;
  runnerd: ControlPlaneComponent;
  ts_ms: number;
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
  seq?: number;
  type: string;
  time: string;
  payload?: any;
};

export type SecretaryMessageRole = "user" | "assistant";

export type SecretaryMessage = {
  id: number;
  time: string;
  role: SecretaryMessageRole;
  content: string;
};

export type SecretaryMessagesResponse = {
  messages: SecretaryMessage[];
};

export type SecretarySendRequest = {
  message: string;
};

export type SecretarySendResponse = {
  reply: string;
};

export type SecretaryThinkingKind = "trace" | "tool_call" | "tool_result" | "error";

export type SecretaryThinkingEvent = {
  kind?: SecretaryThinkingKind;
  step?: number;
  line?: string;
  tool_name?: string;
  ok?: boolean;
  error?: string;
};

export type SecretaryStreamResult = {
  reply: string;
};

export type SecretaryClearResponse = {
  ok: boolean;
};

export type AuditSource =
  | "task_log"
  | "task_trace"
  | "secretary_event"
  | "secretary_compression"
  | "secretary_chat";

export type AuditEntry = {
  id: string;
  source: AuditSource;
  time: string;
  task_id?: string;
  run_id?: string;
  title: string;
  summary: string;
  raw_preview: string;
};

export type AuditEntryDetail = AuditEntry & {
  raw: string;
  meta?: Record<string, any>;
};

export type AuditSourceInfo = {
  source: AuditSource;
  label: string;
  default_enabled: boolean;
  supports_task_id: boolean;
  supports_run_id: boolean;
  supports_streams: boolean;
  default_streams?: Array<"stdout" | "stderr" | "system" | "assistant">;
};

export type AuditQuery = {
  sources?: AuditSource[];
  q?: string;
  from?: string;
  to?: string;
  task_id?: string;
  run_id?: string;
  streams?: Array<"stdout" | "stderr" | "system" | "assistant">;
  limit?: number;
  cursor?: string;
};

export type AuditEntriesResponse = {
  entries: AuditEntry[];
  next_cursor?: string;
};

export type AuditSourcesResponse = {
  sources: AuditSourceInfo[];
};

export type AuditGCSourceResult = {
  source: AuditSource;
  table: string;
  deleted_by_age: number;
  deleted_by_count: number;
  error?: string;
};

export type AuditGCStatus = {
  run_at: string;
  duration_ms: number;
  results: AuditGCSourceResult[];
};

export type AuditRetentionStatus = {
  days: number;
  max_rows_per_source: number;
  gc_interval_seconds: number;
  last_run?: AuditGCStatus;
};

export type AuthFieldStatus = {
  effective: "env" | "stored" | "live" | "default" | "none";
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
  warnings?: string[];
};

export type AuthInfo = {
  status: AuthStatus;
  storage_path: string;
};

export type AuthImportEnvResponse = {
  status: AuthStatus;
  storage_path: string;
  imported: string[];
  skipped: string[];
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

export type ProviderActiveSelection = {
  claude?: string;
  codex?: string;
  secretary?: string;
};

export type ProviderSyncLive = {
  claude?: boolean;
  codex?: boolean;
  secretary?: boolean;
};

export type ProviderClaudeTarget = {
  base_url?: string;
  api_key?: string;
  auth_token?: string;
  model?: string;
  small_fast_model?: string;
};

export type ProviderCodexTarget = {
  base_url?: string;
  api_key?: string;
  model?: string;
  reasoning_effort?: string;
};

export type ProviderSecretaryTarget = {
  backend?: "simple-http";
  simple_http?: {
    base_url?: string;
    api_key?: string;
    auth_token?: string;
    model?: string;
  };
};

export type ProviderTargets = {
  claude?: ProviderClaudeTarget;
  codex?: ProviderCodexTarget;
  secretary?: ProviderSecretaryTarget;
};

export type ProviderProfile = {
  id: string;
  name: string;
  tool?: "claude" | "codex" | "secretary";
  targets: ProviderTargets;
  sync_live?: ProviderSyncLive;
  created_at?: string;
  updated_at?: string;
};

export type ProvidersListResponse = {
  profiles: ProviderProfile[];
  active: ProviderActiveSelection;
  storage_path?: string;
};

export type ProviderUpsertResponse = {
  profile: ProviderProfile;
};

export type ProviderDeleteResponse = {
  ok: boolean;
};

export type ProviderActivateResponse = {
  profile: ProviderProfile;
  active: ProviderActiveSelection;
  auth_status: AuthStatus;
};

export type ProviderSpeedTestResult = {
  url: string;
  ok: boolean;
  status_code?: number;
  latency_ms?: number;
  error?: string;
  hint?: string;
};

export type ProviderSpeedTestResponse = {
  result: ProviderSpeedTestResult;
};

export type ProviderPingTestResult = {
  endpoint: string;
  ok: boolean;
  status_code?: number;
  latency_ms?: number;
  response?: string;
  error?: string;
  hint?: string;
};

export type ProviderPingTestResponse = {
  result: ProviderPingTestResult;
};

export type ProviderImportLiveResponse = {
  profile: ProviderProfile;
  live?: any;
};

export type ProviderImportEnvResponse = {
  profile: ProviderProfile;
  imported: string[];
};

export type ProviderExportResponse = {
  profiles: ProviderProfile[];
  active: ProviderActiveSelection;
  hint?: string;
};

export type ProviderImportResponse = {
  imported: ProviderProfile[];
  count?: number;
};

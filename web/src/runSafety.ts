import type { Task, Tool, ToolDriver } from "./types";

export type TaskIntent = "analyze" | "code" | "search-browse" | "install";

export type SafetyPresetOption = {
  value: string;
  label: string;
  risk: "low" | "med" | "high" | "extreme";
};

export const DEFAULT_RUN_SAFETY_INSTALL_UNLOCK = true;

export type RunSafetyPayload = {
  unsafe_automation?: boolean;
  safety_envelope?: string;
  safety_preset?: string;
  task_intent?: string;
  codex_sandbox?: string;
  codex_approval_policy?: string;
  codex_search?: boolean;
  claude_permission_mode?: string;
  claude_sandbox?: boolean;
};

export function normalizeTaskIntent(raw: string): TaskIntent {
  const v = String(raw ?? "").trim();
  if (v === "analyze" || v === "code" || v === "search-browse" || v === "install") return v;
  return "code";
}

export function safetyPresetsForDriver(driver: ToolDriver): SafetyPresetOption[] {
  if (driver === "codex") {
    return [
      { value: "workspace-write", label: "workspace-write（工作区可写，沙箱）", risk: "low" },
      { value: "read-only", label: "read-only（只读，沙箱）", risk: "low" },
      { value: "search-browse", label: "search-browse（搜索/浏览：开启 web_search）", risk: "med" },
      { value: "danger-full-access", label: "danger-full-access（高风险：可访问工作区外）", risk: "high" },
      { value: "unsafe", label: "unsafe（极高风险：无 sandbox/审批）", risk: "extreme" },
    ];
  }
  if (driver === "claude-code") {
    return [
      { value: "search-browse", label: "search-browse（查资料/浏览：开启 WebFetch/WebSearch）", risk: "med" },
      { value: "no-network", label: "no-network（默认安全：禁 WebFetch/WebSearch，禁 curl/wget）", risk: "low" },
      { value: "unsafe", label: "unsafe（高风险：跳过权限确认，无 bash sandbox）", risk: "high" },
    ];
  }
  return [];
}

export function toolDriverForWorkerType(workerType: string, toolsList?: Tool[]): ToolDriver {
  const id = String(workerType ?? "").trim();
  if (!id) return "exec";
  const tools = Array.isArray(toolsList) ? toolsList : [];
  const t = tools.find((x) => String(x?.id ?? "").trim() === id);
  if (t?.driver) return t.driver;
  if (id === "claude-code") return "claude-code";
  if (id === "codex") return "codex";
  if (id === "exec") return "exec";
  return "exec";
}

export function recommendSafetyPreset(driver: ToolDriver, intent: TaskIntent): string {
  if (driver === "codex") {
    if (intent === "analyze") return "read-only";
    if (intent === "search-browse") return "search-browse";
    if (intent === "install") return "danger-full-access";
    return "workspace-write";
  }
  if (driver === "claude-code") {
    if (intent === "install") return "unsafe";
    // Default to "has network" because WebFetch is low-risk; the real risk is downloading/executing scripts.
    return "search-browse";
  }
  return "";
}

export function inferTaskIntentFromSafetyPreset(driver: ToolDriver, preset: string): TaskIntent {
  const d = String(driver ?? "").trim();
  const p = String(preset ?? "").trim();

  if (d === "codex") {
    if (p === "read-only") return "analyze";
    if (p === "search-browse") return "search-browse";
    if (p === "danger-full-access") return "install";
    if (p === "unsafe") return "install";
    return "code";
  }

  if (d === "claude-code") {
    if (p === "search-browse") return "search-browse";
    if (p === "unsafe") return "install";
    return "code";
  }

  return "code";
}

export function normalizeSafetyPreset(driver: ToolDriver, intent: TaskIntent, raw: string): string {
  const v = String(raw ?? "").trim();
  const allowed = safetyPresetsForDriver(driver).map((p) => p.value);
  if (allowed.includes(v)) return v;
  const rec = recommendSafetyPreset(driver, intent);
  if (allowed.includes(rec)) return rec;
  return allowed[0] ?? "";
}

export function isHighRiskPreset(driver: ToolDriver, preset: string): boolean {
  const p = String(preset ?? "").trim();
  if (driver === "codex") return p === "danger-full-access" || p === "unsafe";
  if (driver === "claude-code") return p === "unsafe";
  return false;
}

export function buildRunSafetyPayload(driver: ToolDriver, intent: TaskIntent, preset: string): RunSafetyPayload {
  const sp = String(preset ?? "").trim();
  const ti = normalizeTaskIntent(intent);

  if (driver === "codex") {
    if (sp === "unsafe") {
      return {
        unsafe_automation: true,
        safety_preset: sp,
        task_intent: ti,
      };
    }
    const sandbox =
      sp === "search-browse"
        ? "workspace-write"
        : sp === "read-only" || sp === "workspace-write" || sp === "danger-full-access"
          ? sp
          : "workspace-write";
    return {
      safety_preset: sp,
      task_intent: ti,
      codex_sandbox: sandbox,
      codex_approval_policy: "untrusted",
      codex_search: sp === "search-browse" || undefined,
    };
  }

  if (driver === "claude-code") {
    if (sp === "unsafe") {
      return {
        unsafe_automation: true,
        safety_preset: sp,
        task_intent: ti,
        // Disable Claude bash sandbox in unsafe mode so the run can access system network
        // (e.g. pip/python/curl) when the user explicitly opts into high risk.
        claude_sandbox: false,
      };
    }
    return {
      safety_preset: sp,
      task_intent: ti,
      // Non-interactive runs cannot click approval prompts; accept file edits by default.
      claude_permission_mode: "acceptEdits",
      claude_sandbox: true,
    };
  }

  return {};
}

export function effectiveSafetyPresetForTask(driver: ToolDriver, task: Task): string {
  const explicit = String(task.safety_preset ?? "").trim();
  if (explicit) return explicit;
  if (task.unsafe_automation) return "unsafe";

  if (driver === "codex") {
    if (task.codex_search) return "search-browse";
    const sb = String(task.codex_sandbox ?? "").trim();
    if (sb) return sb;
    return "workspace-write";
  }
  if (driver === "claude-code") {
    if (String(task.task_intent ?? "").trim() === "search-browse") return "search-browse";
    if ((task.claude_webfetch_domains ?? []).length > 0) return "search-browse";
    // Default to "has network" to match the recommended preset.
    return "search-browse";
  }
  return "";
}

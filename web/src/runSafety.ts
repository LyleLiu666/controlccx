import type { Task, ToolDriver } from "./types";

export type TaskIntent = "analyze" | "code" | "search-browse" | "install";

export type SafetyPresetOption = {
  value: string;
  label: string;
  risk: "low" | "med" | "high" | "extreme";
};

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
      { value: "workspace-write", label: "Workspace write (sandboxed)", risk: "low" },
      { value: "read-only", label: "Read-only (sandboxed)", risk: "low" },
      { value: "search-browse", label: "Search/browse (sandbox + web search)", risk: "med" },
      { value: "danger-full-access", label: "Full access (sandbox: danger-full-access)", risk: "high" },
      { value: "unsafe", label: "UNSAFE (no sandbox/approvals)", risk: "extreme" },
    ];
  }
  if (driver === "claude-code") {
    return [
      { value: "search-browse", label: "Search/browse (WebFetch enabled)", risk: "med" },
      { value: "no-network", label: "Sandboxed (no network)", risk: "low" },
      { value: "unsafe", label: "UNSAFE (skip permissions)", risk: "high" },
    ];
  }
  return [];
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
      codex_approval_policy: "never",
      codex_search: sp === "search-browse" || undefined,
    };
  }

  if (driver === "claude-code") {
    if (sp === "unsafe") {
      // Keep sandbox enabled (when supported) even in unsafe mode.
      return {
        unsafe_automation: true,
        safety_preset: sp,
        task_intent: ti,
        claude_sandbox: true,
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

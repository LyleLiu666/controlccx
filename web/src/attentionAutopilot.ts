export const ATTENTION_AUTOPILOT_STOP_VALUE = "stop";

export type AttentionAutopilotAttemptInput = {
  enabled: boolean;
  deleted: boolean;
  hasSessionID: boolean;
  sessionStatus: string;
  latestStatus: string;
  nowMs: number;
  lastAttemptMs: number;
  cooldownMs: number;
};

export function attentionAutopilotSeenAtMs(
  seen: Record<string, string> | null | undefined,
  sessionKey: string,
): number {
  const key = String(sessionKey ?? "").trim();
  if (!key) return 0;
  const raw = String(seen?.[key] ?? "").trim();
  if (!raw) return 0;
  if (raw === ATTENTION_AUTOPILOT_STOP_VALUE) return Number.POSITIVE_INFINITY;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) ? n : 0;
}

export function attentionAutopilotMarkSeen(
  seen: Record<string, string> | null | undefined,
  sessionKey: string,
  nowMs = Date.now(),
): Record<string, string> {
  const key = String(sessionKey ?? "").trim();
  if (!key) return { ...(seen ?? {}) };
  const next: Record<string, string> = { ...(seen ?? {}) };
  next[key] = String(nowMs);
  return next;
}

export function attentionAutopilotStopForSession(
  seen: Record<string, string> | null | undefined,
  sessionKey: string,
): Record<string, string> {
  const key = String(sessionKey ?? "").trim();
  if (!key) return { ...(seen ?? {}) };
  const next: Record<string, string> = { ...(seen ?? {}) };
  next[key] = ATTENTION_AUTOPILOT_STOP_VALUE;
  return next;
}

export function attentionAutopilotShouldAttempt(
  input: AttentionAutopilotAttemptInput,
): boolean {
  if (!input.enabled) return false;
  if (input.deleted) return false;
  if (!input.hasSessionID) return false;
  if (input.sessionStatus !== "interrupted") return false;
  if (
    input.latestStatus === "running" ||
    input.latestStatus === "queued" ||
    input.latestStatus === "waiting" ||
    input.latestStatus === "awaiting_approval" ||
    input.latestStatus === "blocked"
  )
    return false;
  if (input.lastAttemptMs === Number.POSITIVE_INFINITY) return false;
  if (
    input.lastAttemptMs > 0 &&
    input.nowMs-input.lastAttemptMs < input.cooldownMs
  )
    return false;
  return true;
}

export function attentionAutopilotIsNoConversationFound(message: string): boolean {
  const m = String(message ?? "").toLowerCase();
  if (!m.includes("no conversation found")) return false;
  return m.includes("session");
}

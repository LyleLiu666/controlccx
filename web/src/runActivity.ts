import { prettifyLogMessage } from "./logPretty.ts";

export type LogLike = {
  time: string;
  stream: string;
  message?: string | null;
};

export type RunActivity = {
  time: string;
  stream: string;
  summary: string;
};

function timeMs(t: string): number {
  const ms = Date.parse(String(t ?? ""));
  return Number.isFinite(ms) ? ms : 0;
}

function isNoiseSummary(summary: string): boolean {
  const s = String(summary ?? "").trim().toLowerCase();
  return s.startsWith("run.start") || s.startsWith("run.finish");
}

export function deriveRunActivity(logs: LogLike[]): RunActivity | null {
  if (!Array.isArray(logs) || logs.length === 0) return null;

  const sorted = logs
    .slice()
    .sort((a, b) => timeMs(b.time) - timeMs(a.time));

  let fallback: RunActivity | null = null;
  for (const l of sorted) {
    const raw = String(l.message ?? "").trim();
    if (!raw) continue;
    const pretty = prettifyLogMessage(raw);
    const summary = String(pretty.summary ?? "").trim();
    if (!summary) continue;
    const activity: RunActivity = { time: l.time, stream: l.stream, summary };
    if (!fallback) fallback = activity;
    if (isNoiseSummary(summary)) continue;
    return activity;
  }

  return fallback;
}

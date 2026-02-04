export type LogLike = {
  time?: string | null;
  message?: string | null;
};

export type RunUsageModel = {
  model: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadInputTokens: number;
  cacheCreationInputTokens: number;
  cachedInputTokens: number;
  totalTokens: number;
  costUSD?: number;
  contextWindow?: number;
  maxOutputTokens?: number;
};

export type RunUsageSummary = {
  source: "result" | "turn.completed" | "message.usage";
  inputTokens: number;
  outputTokens: number;
  cacheReadInputTokens: number;
  cacheCreationInputTokens: number;
  cachedInputTokens: number;
  inputTotalTokens: number;
  totalTokens: number;
  totalCostUSD?: number;
  models?: RunUsageModel[];
};

type UsageParts = Omit<
  RunUsageSummary,
  "source" | "inputTotalTokens" | "totalTokens" | "models" | "totalCostUSD"
>;

function timeMs(t: string): number {
  const ms = Date.parse(String(t ?? ""));
  return Number.isFinite(ms) ? ms : 0;
}

function safeJsonParse(text: string): any | null {
  const raw = String(text ?? "").trim();
  if (!raw) return null;
  if (!(raw.startsWith("{") || raw.startsWith("["))) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function toPosInt(v: any): number {
  const n = typeof v === "number" ? v : Number(v);
  if (!Number.isFinite(n)) return 0;
  const i = Math.floor(n);
  return i > 0 ? i : 0;
}

function toPosFloat(v: any): number | undefined {
  const n = typeof v === "number" ? v : Number(v);
  if (!Number.isFinite(n)) return undefined;
  if (n <= 0) return undefined;
  return n;
}

function normalizeUsage(raw: any): UsageParts {
  return {
    inputTokens: toPosInt(raw?.input_tokens ?? raw?.inputTokens),
    outputTokens: toPosInt(raw?.output_tokens ?? raw?.outputTokens),
    cacheReadInputTokens: toPosInt(raw?.cache_read_input_tokens ?? raw?.cacheReadInputTokens),
    cacheCreationInputTokens: toPosInt(
      raw?.cache_creation_input_tokens ?? raw?.cacheCreationInputTokens,
    ),
    cachedInputTokens: toPosInt(raw?.cached_input_tokens ?? raw?.cachedInputTokens),
  };
}

function addUsage(a: UsageParts, b: UsageParts): UsageParts {
  return {
    inputTokens: a.inputTokens + b.inputTokens,
    outputTokens: a.outputTokens + b.outputTokens,
    cacheReadInputTokens: a.cacheReadInputTokens + b.cacheReadInputTokens,
    cacheCreationInputTokens: a.cacheCreationInputTokens + b.cacheCreationInputTokens,
    cachedInputTokens: a.cachedInputTokens + b.cachedInputTokens,
  };
}

function buildSummary(
  source: RunUsageSummary["source"],
  usage: UsageParts,
  opts?: { totalCostUSD?: number; models?: RunUsageModel[] },
): RunUsageSummary | null {
  const inputTotalTokens =
    usage.inputTokens +
    usage.cacheReadInputTokens +
    usage.cacheCreationInputTokens +
    usage.cachedInputTokens;
  const totalTokens = inputTotalTokens + usage.outputTokens;

  if (!totalTokens) return null;

  const out: RunUsageSummary = {
    source,
    ...usage,
    inputTotalTokens,
    totalTokens,
  };
  if (typeof opts?.totalCostUSD === "number" && Number.isFinite(opts.totalCostUSD)) {
    out.totalCostUSD = opts.totalCostUSD;
  }
  if (opts?.models?.length) out.models = opts.models;
  return out;
}

function normalizeModelUsage(raw: any): RunUsageModel[] {
  if (!raw || typeof raw !== "object") return [];

  const out: RunUsageModel[] = [];
  for (const [model, v] of Object.entries(raw)) {
    if (!v || typeof v !== "object") continue;
    const parts = normalizeUsage(v);
    const inputTotalTokens =
      parts.inputTokens +
      parts.cacheReadInputTokens +
      parts.cacheCreationInputTokens +
      parts.cachedInputTokens;
    const totalTokens = inputTotalTokens + parts.outputTokens;
    if (!totalTokens) continue;

    const costUSD = toPosFloat((v as any).costUSD ?? (v as any).cost_usd);
    const contextWindow = toPosInt((v as any).contextWindow ?? (v as any).context_window) || undefined;
    const maxOutputTokens = toPosInt((v as any).maxOutputTokens ?? (v as any).max_output_tokens) || undefined;

    out.push({
      model: String(model),
      ...parts,
      totalTokens,
      costUSD,
      contextWindow,
      maxOutputTokens,
    });
  }

  out.sort((a, b) => {
    const cost = (b.costUSD ?? 0) - (a.costUSD ?? 0);
    if (cost) return cost;
    return b.totalTokens - a.totalTokens;
  });
  return out;
}

export function deriveRunUsage(logs: LogLike[]): RunUsageSummary | null {
  if (!Array.isArray(logs) || logs.length === 0) return null;

  const sorted = logs
    .slice()
    .sort((a, b) => timeMs(String(b.time ?? "")) - timeMs(String(a.time ?? "")));

  for (const l of sorted) {
    const parsed = safeJsonParse(String(l.message ?? ""));
    if (!parsed || typeof parsed !== "object") continue;
    if (String((parsed as any).type ?? "") !== "result") continue;

    const usage = (parsed as any).usage;
    const parts = normalizeUsage(usage);
    const models = normalizeModelUsage((parsed as any).modelUsage);
    const totalCostUSD = toPosFloat((parsed as any).total_cost_usd);
    const summary = buildSummary("result", parts, { totalCostUSD, models });
    if (summary) return summary;
  }

  let turnSum: UsageParts = {
    inputTokens: 0,
    outputTokens: 0,
    cacheReadInputTokens: 0,
    cacheCreationInputTokens: 0,
    cachedInputTokens: 0,
  };
  let hasTurnUsage = false;

  for (const l of logs) {
    const parsed = safeJsonParse(String(l.message ?? ""));
    if (!parsed || typeof parsed !== "object") continue;
    if (String((parsed as any).type ?? "") !== "turn.completed") continue;
    if (!(parsed as any).usage) continue;
    hasTurnUsage = true;
    turnSum = addUsage(turnSum, normalizeUsage((parsed as any).usage));
  }

  const turnSummary = hasTurnUsage ? buildSummary("turn.completed", turnSum) : null;
  if (turnSummary) return turnSummary;

  let msgSum: UsageParts = {
    inputTokens: 0,
    outputTokens: 0,
    cacheReadInputTokens: 0,
    cacheCreationInputTokens: 0,
    cachedInputTokens: 0,
  };
  let hasMsgUsage = false;

  for (const l of logs) {
    const parsed = safeJsonParse(String(l.message ?? ""));
    if (!parsed || typeof parsed !== "object") continue;
    const usage = (parsed as any)?.message?.usage;
    if (!usage) continue;
    hasMsgUsage = true;
    msgSum = addUsage(msgSum, normalizeUsage(usage));
  }

  const msgSummary = hasMsgUsage ? buildSummary("message.usage", msgSum) : null;
  if (msgSummary) return msgSummary;

  return null;
}


export type PrettyLog = {
  kind: "json" | "text";
  summary: string;
  details: string;
  prettyJson?: string;
};

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

function pickFirstContentItem(message: any): any | null {
  const c = message?.content;
  if (!Array.isArray(c) || c.length === 0) return null;
  return c[0] ?? null;
}

function summarizeFromMessage(message: any): string {
  const role = String(message?.role ?? "").trim();
  const model = String(message?.model ?? "").trim();
  const first = pickFirstContentItem(message);
  const partType = String(first?.type ?? "").trim();
  const name = String(first?.name ?? "").trim();
  const toolUseID = String(first?.tool_use_id ?? first?.id ?? "").trim();

  const parts: string[] = [];
  if (role) parts.push(role);
  if (partType) {
    if (partType === "tool_use") {
      parts.push(name ? `tool_use ${name}` : "tool_use");
      if (toolUseID) parts.push(shortID(toolUseID));
    } else if (partType === "tool_result") {
      parts.push("tool_result");
      if (toolUseID) parts.push(shortID(toolUseID));
    } else {
      parts.push(partType);
    }
  }
  if (!parts.length && model) parts.push(model);
  return parts.join(" · ");
}

function shortID(id: string): string {
  const s = (id ?? "").trim();
  if (s.length <= 14) return s;
  const lower = s.toLowerCase();
  // Tool-call ids benefit from a slightly longer prefix for disambiguation.
  if (lower.startsWith("call_")) return s.slice(0, 13);
  if (lower.startsWith("msg_")) return s.slice(0, 12);
  return s.slice(0, 8);
}

function summarizeJson(obj: any): string {
  const t = String(obj?.type ?? "").trim();
  const subtype = String(obj?.subtype ?? obj?.hook_event ?? "").trim();
  const message = obj?.message;
  const role = String(message?.role ?? "").trim();

  const parts: string[] = [];
  if (t && !(role && t === role)) parts.push(t);
  if (subtype && subtype !== t) parts.push(subtype);

  const msgSummary = summarizeFromMessage(message);
  if (msgSummary) parts.push(msgSummary);

  return parts.join(" · ") || "json";
}

function stringifyPretty(obj: any): string {
  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
}

export function prettifyLogMessage(rawMessage: string): PrettyLog {
  const raw = String(rawMessage ?? "");
  const parsed = safeJsonParse(raw);
  if (!parsed) {
    const oneLine = raw.replace(/\s+/g, " ").trim();
    const summary = oneLine.length > 140 ? `${oneLine.slice(0, 140)}…` : oneLine;
    return {
      kind: "text",
      summary: summary || "(empty)",
      details: raw,
    };
  }

  const pretty = stringifyPretty(parsed);
  return {
    kind: "json",
    summary: summarizeJson(parsed),
    details: pretty,
    prettyJson: pretty,
  };
}

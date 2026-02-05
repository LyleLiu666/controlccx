import type { ToolDriver } from "./types";

function normalizeSkillName(name: string): string {
  return String(name ?? "").trim();
}

export function formatSkillToken(name: string, driver: ToolDriver): string {
  const skill = normalizeSkillName(name);
  if (!skill) return "";

  switch (driver) {
    case "claude-code":
      return `/${skill}`;
    case "codex":
      return `$${skill}`;
    default:
      return `$${skill}`;
  }
}

function isWhitespace(ch: string): boolean {
  return ch === " " || ch === "\t" || ch === "\n" || ch === "\r";
}

function isNameStart(ch: string): boolean {
  return /^[A-Za-z0-9]$/.test(ch);
}

function isNameChar(ch: string): boolean {
  return /^[A-Za-z0-9@._-]$/.test(ch);
}

export function extractSkillTokenNames(prompt: string): string[] {
  const text = String(prompt ?? "");
  if (!text) return [];

  const out: string[] = [];
  const seen = new Set<string>();

  for (let i = 0; i < text.length; i++) {
    const prefix = text[i];
    if (prefix !== "/" && prefix !== "$") continue;

    if (i > 0 && !isWhitespace(text[i - 1])) continue;

    const nameStart = i + 1;
    if (nameStart >= text.length || !isNameStart(text[nameStart])) continue;

    let nameEnd = nameStart;
    while (nameEnd < text.length && isNameChar(text[nameEnd])) nameEnd++;
    const name = text.slice(nameStart, nameEnd);
    if (!name || name.includes("..")) continue;

    if (nameEnd < text.length && !isWhitespace(text[nameEnd])) continue;

    if (!seen.has(name)) {
      seen.add(name);
      out.push(name);
    }
    i = Math.max(i, nameEnd - 1);
  }

  return out;
}

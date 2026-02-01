export type SkillInstallKind = "local" | "git" | "unknown";

function looksLikeGitURL(raw: string): boolean {
  const s = raw.trim();
  if (!s) return false;

  // Common URL forms.
  if (/^(https?|ssh):\/\//i.test(s)) return true;
  if (/^git@[^:]+:.+/i.test(s)) return true; // scp-like: git@host:owner/repo(.git)
  if (/\.git(?:#|$)/i.test(s)) return true;

  return false;
}

function looksLikeLocalPath(raw: string): boolean {
  const s = raw.trim();
  if (!s) return false;

  // POSIX-ish.
  if (s.startsWith("/") || s.startsWith("~")) return true;
  if (s.startsWith("./") || s.startsWith("../")) return true;

  // Windows-ish.
  if (/^[a-zA-Z]:[\\/]/.test(s)) return true;
  if (s.startsWith("\\\\")) return true;

  // file:// URL.
  if (/^file:\/\//i.test(s)) return true;

  return false;
}

export function detectSkillInstallKind(input: string): SkillInstallKind {
  const s = String(input ?? "").trim();
  if (!s) return "unknown";
  if (looksLikeGitURL(s)) return "git";
  if (looksLikeLocalPath(s)) return "local";
  return "unknown";
}

export function normalizeGitRepoURL(input: string): string {
  const s = String(input ?? "").trim();
  if (!s) return "";

  // Pass-through for full URLs and scp-like forms.
  if (looksLikeGitURL(s)) return s;

  // Convenience: allow "owner/repo" shorthand for GitHub.
  // Avoid treating relative paths like "./a/b" or "../a/b" as GitHub.
  if (/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(s)) {
    return `https://github.com/${s}`;
  }

  return s;
}


import hljs from "highlight.js/lib/common";

export type HighlightReason =
  | ""
  | "empty"
  | "plaintext"
  | "too_large"
  | "auto_too_large"
  | "error";

export type HighlightResult = {
  html: string;
  highlighted: boolean;
  lang: string | null;
  reason: HighlightReason;
};

export type Highlighter = {
  getLanguage: (lang: string) => any;
  highlight: (
    text: string,
    opts: { language: string; ignoreIllegals?: boolean },
  ) => { value: string };
  highlightAuto: (text: string) => { value: string };
};

const MAX_HIGHLIGHT_CHARS = 120_000;
const MAX_AUTO_HIGHLIGHT_CHARS = 20_000;

function escapeHtml(s: string): string {
  return (s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export function highlightLangFromPath(path: string): string | null {
  const p = (path ?? "").toLowerCase();
  if (p.endsWith(".ts") || p.endsWith(".mts") || p.endsWith(".cts")) return "typescript";
  if (p.endsWith(".tsx")) return "tsx";
  if (p.endsWith(".js") || p.endsWith(".mjs") || p.endsWith(".cjs")) return "javascript";
  if (p.endsWith(".jsx")) return "jsx";
  if (p.endsWith(".vue")) return "xml";
  if (p.endsWith(".json")) return "json";
  if (p.endsWith(".yaml") || p.endsWith(".yml")) return "yaml";
  if (p.endsWith(".toml")) return "toml";
  if (p.endsWith(".md") || p.endsWith(".markdown")) return "markdown";
  if (p.endsWith(".go")) return "go";
  if (p.endsWith(".py")) return "python";
  if (p.endsWith(".sh") || p.endsWith(".bash") || p.endsWith(".zsh")) return "bash";
  if (p.endsWith(".ps1")) return "powershell";
  if (p.endsWith(".sql")) return "sql";
  if (p.endsWith(".css")) return "css";
  if (p.endsWith(".html") || p.endsWith(".htm")) return "xml";
  if (p.endsWith(".xml")) return "xml";
  if (p.endsWith(".diff") || p.endsWith(".patch")) return "diff";
  if (p.endsWith(".txt") || p.endsWith(".log")) return "plaintext";
  return null;
}

export function highlightCodeForPreview(
  text: string,
  path: string,
  highlighter: Highlighter = hljs,
): HighlightResult {
  const raw = String(text ?? "");
  if (!raw) return { html: "", highlighted: false, lang: null, reason: "empty" };

  if (raw.length > MAX_HIGHLIGHT_CHARS) {
    return {
      html: escapeHtml(raw),
      highlighted: false,
      lang: null,
      reason: "too_large",
    };
  }

  const lang = highlightLangFromPath(path);
  if (lang === "plaintext") {
    return { html: escapeHtml(raw), highlighted: false, lang, reason: "plaintext" };
  }

  try {
    if (lang && highlighter.getLanguage(lang)) {
      return {
        html: highlighter.highlight(raw, { language: lang, ignoreIllegals: true }).value,
        highlighted: true,
        lang,
        reason: "",
      };
    }
    if (raw.length > MAX_AUTO_HIGHLIGHT_CHARS) {
      return { html: escapeHtml(raw), highlighted: false, lang: null, reason: "auto_too_large" };
    }
    return {
      html: highlighter.highlightAuto(raw).value,
      highlighted: true,
      lang: null,
      reason: "",
    };
  } catch {
    return { html: escapeHtml(raw), highlighted: false, lang, reason: "error" };
  }
}

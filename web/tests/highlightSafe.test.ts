import assert from "node:assert/strict";
import test from "node:test";

import { highlightCodeForPreview } from "../src/highlight.ts";

function fakeHighlighter() {
  const calls: string[] = [];
  return {
    calls,
    hl: {
      getLanguage: (lang: string) => {
        calls.push(`getLanguage:${lang}`);
        return lang === "go" ? {} : null;
      },
      highlight: (text: string, opts: { language: string }) => {
        calls.push(`highlight:${opts.language}:${text.length}`);
        return { value: `<span>${opts.language}</span>` };
      },
      highlightAuto: (text: string) => {
        calls.push(`highlightAuto:${text.length}`);
        return { value: "<span>auto</span>" };
      },
    },
  };
}

test("highlightCodeForPreview skips highlighting when text is huge", () => {
  const { calls, hl } = fakeHighlighter();
  const huge = "x".repeat(220_000);
  const res = highlightCodeForPreview(huge, "main.go", hl as any);
  assert.equal(res.highlighted, false);
  assert.equal(res.reason, "too_large");
  assert.deepEqual(calls, []);
});

test("highlightCodeForPreview avoids highlightAuto for unknown languages when large", () => {
  const { calls, hl } = fakeHighlighter();
  const large = "x".repeat(80_000);
  const res = highlightCodeForPreview(large, "README.unknown", hl as any);
  assert.equal(res.highlighted, false);
  assert.equal(res.reason, "auto_too_large");
  assert.deepEqual(calls, []);
});

test("highlightCodeForPreview highlights known languages when small", () => {
  const { calls, hl } = fakeHighlighter();
  const small = "package main\n";
  const res = highlightCodeForPreview(small, "main.go", hl as any);
  assert.equal(res.highlighted, true);
  assert.equal(res.lang, "go");
  assert.match(res.html, /go/);
  assert.deepEqual(calls, ["getLanguage:go", `highlight:go:${small.length}`]);
});

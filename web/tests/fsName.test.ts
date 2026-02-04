import test from "node:test";
import assert from "node:assert/strict";
import { validateNewFolderName } from "../src/fsName.ts";

test("validateNewFolderName rejects empty/invalid names", () => {
  assert.deepEqual(validateNewFolderName(""), { ok: false, error: "Folder name is required." });
  assert.deepEqual(validateNewFolderName("   "), { ok: false, error: "Folder name is required." });
  assert.deepEqual(validateNewFolderName("."), {
    ok: false,
    error: "Folder name cannot be '.' or '..'.",
  });
  assert.deepEqual(validateNewFolderName(".."), {
    ok: false,
    error: "Folder name cannot be '.' or '..'.",
  });
  assert.deepEqual(validateNewFolderName("a/b"), {
    ok: false,
    error: "Folder name cannot include path separators.",
  });
  assert.deepEqual(validateNewFolderName("a\\b"), {
    ok: false,
    error: "Folder name cannot include path separators.",
  });
  assert.deepEqual(validateNewFolderName("a\0b"), {
    ok: false,
    error: "Folder name contains invalid characters.",
  });
});

test("validateNewFolderName trims and accepts normal names", () => {
  assert.deepEqual(validateNewFolderName("  demo  "), { ok: true, name: "demo" });
  assert.deepEqual(validateNewFolderName(".hidden"), { ok: true, name: ".hidden" });
});

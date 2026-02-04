export type FolderNameValidation =
  | { ok: true; name: string }
  | { ok: false; error: string };

export function validateNewFolderName(raw: string): FolderNameValidation {
  const name = String(raw ?? "").trim();
  if (!name) return { ok: false, error: "Folder name is required." };
  if (name === "." || name === "..") return { ok: false, error: "Folder name cannot be '.' or '..'." };
  if (name.includes("\0")) return { ok: false, error: "Folder name contains invalid characters." };
  if (name.includes("/") || name.includes("\\")) {
    return { ok: false, error: "Folder name cannot include path separators." };
  }
  return { ok: true, name };
}


export function isNoConversationFound(message: string): boolean {
  const m = String(message ?? "").toLowerCase();
  if (!m.includes("no conversation found")) return false;
  return m.includes("session");
}


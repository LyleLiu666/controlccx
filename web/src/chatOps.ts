import type { ChatMessage } from "./types";

function fnv1a32Hex(input: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i);
    // 32-bit FNV prime multiplication with overflow.
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16).padStart(8, "0");
}

export function buildChatIdempotencyKey(input: {
  after: number;
  message: string;
  backend: string;
  maxSteps: number;
}): string {
  const after = Number.isFinite(input.after) ? Math.max(0, Math.floor(input.after)) : 0;
  const backend = String(input.backend ?? "").trim() || "auto";
  const maxSteps = Number.isFinite(input.maxSteps) ? Math.max(1, Math.floor(input.maxSteps)) : 8;
  const msg = String(input.message ?? "").trim();
  const digest = fnv1a32Hex(`${backend}|${maxSteps}|${msg}`);
  return `chat:${after}:${digest}`;
}

export function appendChatMessageUnique(
  list: ChatMessage[],
  msg: ChatMessage,
): ChatMessage[] {
  if (!msg || typeof msg.id !== "number") return list;
  if (list.some((m) => m.id === msg.id)) return list;
  return [...list, msg];
}

type SendChatFn = (message: string) => Promise<{ message: string }>;
type FetchChatFn = (after?: number, limit?: number) => Promise<ChatMessage[]>;

export async function sendChatAndReload(
  message: string,
  deps: { sendChat: SendChatFn; fetchChat: FetchChatFn; after?: number; limit?: number },
): Promise<ChatMessage[]> {
  await deps.sendChat(message);
  return deps.fetchChat(deps.after ?? 0, deps.limit ?? 200);
}

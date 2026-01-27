import type { ChatMessage } from "./types";

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


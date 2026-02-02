import type { Task } from "./types";
import { attentionAutopilotIsNoConversationFound } from "./attentionAutopilot.ts";

export function shouldSkipAutoDeliveryForemanForTask(
  task: Pick<Task, "status" | "error" | "warning">,
): boolean {
  const status = String(task.status ?? "");
  if (status === "blocked") return true;
  if (status !== "failed") return false;
  return (
    attentionAutopilotIsNoConversationFound(task.error ?? "") ||
    attentionAutopilotIsNoConversationFound(task.warning ?? "")
  );
}

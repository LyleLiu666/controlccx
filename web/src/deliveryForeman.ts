import type { Task } from "./types";
import { isNoConversationFound } from "./noConversationFound.ts";

export function shouldSkipAutoDeliveryForemanForTask(
  task: Pick<Task, "status" | "error" | "warning">,
): boolean {
  const status = String(task.status ?? "");
  if (status === "blocked") return true;
  if (status !== "failed") return false;
  return (
    isNoConversationFound(task.error ?? "") ||
    isNoConversationFound(task.warning ?? "")
  );
}

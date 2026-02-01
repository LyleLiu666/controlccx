import type { Task } from "./types";
import { attentionAutopilotIsNoConversationFound } from "./attentionAutopilot.ts";

export type ResumeOrigin = "manual" | "autopilot" | "";

export function shouldOfferRehydrateForTask(
  task: Pick<Task, "worker_type" | "mode" | "status" | "error" | "warning">,
  origin: ResumeOrigin,
): boolean {
  if (origin !== "manual") return false;
  if (task.worker_type !== "claude-code") return false;
  if (task.mode !== "resume") return false;
  if (task.status !== "failed") return false;
  return (
    attentionAutopilotIsNoConversationFound(task.error ?? "") ||
    attentionAutopilotIsNoConversationFound(task.warning ?? "")
  );
}

import type { Task } from "./types";
import { isNoConversationFound } from "./noConversationFound.ts";

export type ResumeOrigin = "manual" | "";

export function shouldOfferRehydrateForTask(
  task: Pick<Task, "worker_type" | "mode" | "status" | "error" | "warning">,
  origin: ResumeOrigin,
): boolean {
  // Offer rehydrate for "manual" and unknown origins (e.g. runs created outside the UI).
  if (task.worker_type !== "claude-code") return false;
  if (task.mode !== "resume") return false;
  if (task.status !== "failed") return false;
  return (
    isNoConversationFound(task.error ?? "") ||
    isNoConversationFound(task.warning ?? "")
  );
}

import type { Task } from "./types";

export function shouldDismissRunLaunchMask(task: Pick<Task, "status" | "started_at">): boolean {
  if (task.started_at) return true;
  return task.status !== "queued";
}

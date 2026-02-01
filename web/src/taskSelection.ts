export function deriveNextSelectedTaskId(opts: {
  current: string;
  candidates: Array<{ id: string }>;
  autoSelectFirst: boolean;
}): string {
  const current = String(opts.current ?? "").trim();
  const candidates = Array.isArray(opts.candidates) ? opts.candidates : [];
  const auto = !!opts.autoSelectFirst;

  const first = String(candidates[0]?.id ?? "").trim();
  if (!current) return auto ? first : "";

  const exists = candidates.some((t) => String(t?.id ?? "").trim() === current);
  if (exists) return current;

  return first;
}


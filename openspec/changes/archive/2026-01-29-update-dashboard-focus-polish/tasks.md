## 1. Dashboard UX Polish (Frontend)
- [x] 1.1 Sessions list: make compact cards readable (workdir label + prompt summary with tooltips; avoid awkward clamping/blank space)
- [x] 1.2 Session Detail header: show current run instruction (mode + prompt summary) in a single-line, ellipsized way
- [x] 1.3 Narrow layout: ensure Sessions/Detail remain usable under small widths (no broken wrapping, truncation works)

## 2. Verification
- [x] 2.1 Run `openspec validate update-dashboard-focus-polish --strict --no-interactive`
- [x] 2.2 Run `pnpm -C web build` and `pnpm build`
- [x] 2.3 Run `pnpm smoke`

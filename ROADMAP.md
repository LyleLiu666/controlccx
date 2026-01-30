# ControlCCX Roadmap

This repo uses OpenSpec for detailed change proposals, but `openspec/` is intentionally gitignored.
This `ROADMAP.md` is the tracked, public snapshot of “what’s left” in dependency order (地基 → 上层).

Last updated: 2026-01-30

## Status

### Done (archived in OpenSpec)
- ✅ `update-skills-files-pages` — `/skills` + `/files` pages; default open in new tab
- ✅ `add-skill-versions` — Skills versions store + API + UI
- ✅ `add-skills-hub-governance` — tool adapters, onboarding/import/install/update + UI

### In progress
- 🟡 `add-acceptance-gates` — persist acceptance state + UI surface (iteration/report) (9/22 tasks)
- 🟡 `refactor-web-src-structure` — split `web/src/App.vue` into mid-grain components/composables (9/18 tasks)

## Next (foundation → upper layers)

### 1) `add-acceptance-gates` (quality + parallelism foundation)
Goal: for complex runs, the Secretary enforces acceptance with objective vs subjective criteria, persists progress, and auto-iterates (≤10) until accepted or escalated.

Next implementation steps:
- Embed acceptance prompt templates (binary-shipped) and tighten Delivery Foreman instructions
- Add deterministic helpers for objective checks (at minimum: word/section counts) + tests
- Implement/validate the remediation loop contract (iteration i/10 + progress updates + stop/escale rules)
- Improve “never feel stuck” UX: acceptance auto-refresh while running

### 2) `refactor-web-src-structure` (maintainability foundation)
Goal: reduce regression risk and make future UI changes cheaper for humans + AI.

Next implementation steps:
- Extract Live UI into `web/src/components/LiveDrawer.vue`
- Extract Files UI into a component (keep `/files` page behavior)
- Add composables: `useLiveFeed.ts`, `useTasks.ts` and wire them in
- Run the manual regression checklist (Skills / Secretary / Live / Files / Tools / Runs)

### 3) Upper layers (queue)
These are valuable, but should follow the foundations above:
- `add-context-and-templates`
- `add-quick-actions`
- `add-file-advanced-capabilities`
- `add-editor-monaco`


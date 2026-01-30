# ControlCCX Roadmap

This repo uses OpenSpec for detailed change proposals, but `openspec/` is intentionally gitignored.
This `ROADMAP.md` is the tracked, public snapshot of “what’s left” in dependency order (地基 → 上层).

Last updated: 2026-01-30

## Status

### Done (archived in OpenSpec)
- ✅ `update-skills-files-pages` — `/skills` + `/files` pages; default open in new tab
- ✅ `add-skill-versions` — Skills versions store + API + UI
- ✅ `add-skills-hub-governance` — tool adapters, onboarding/import/install/update + UI
- ✅ `add-acceptance-gates` — acceptance state + deterministic helpers + UI progress/report

### In progress
- 🟡 `refactor-web-src-structure` — split `web/src/App.vue` into mid-grain components/composables (9/18 tasks)

## Next (foundation → upper layers)

### 1) `refactor-web-src-structure` (maintainability foundation)
Goal: reduce regression risk and make future UI changes cheaper for humans + AI.

Next implementation steps:
- Extract Live UI into `web/src/components/LiveDrawer.vue`
- Extract Files UI into a component (keep `/files` page behavior)
- Add composables: `useLiveFeed.ts`, `useTasks.ts` and wire them in
- Run the manual regression checklist (Skills / Secretary / Live / Files / Tools / Runs)

### 2) Upper layers (queue)
These are valuable, but should follow the foundations above:
- `add-context-and-templates`
- `add-quick-actions`
- `add-file-advanced-capabilities`
- `add-editor-monaco`

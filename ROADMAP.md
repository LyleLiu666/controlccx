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
- ✅ `refactor-web-src-structure` — split `web/src/App.vue` into mid-grain components/composables

### In progress
- (none)

## Next (foundation → upper layers)

### 1) `add-sandbox-presets` (safety foundation)
Goal: make “safe-by-default” run/resume configuration understandable and 1-click.

### 2) Upper layers (queue)
These are valuable, but should follow the foundations above:
- `add-context-and-templates`
- `add-quick-actions`
- `add-file-advanced-capabilities`
- `add-editor-monaco`

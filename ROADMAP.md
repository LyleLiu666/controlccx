# ControlCCX Roadmap

This repo uses OpenSpec for detailed change proposals, but `openspec/` is intentionally gitignored.
This `ROADMAP.md` is the tracked, public snapshot of “what’s left” in dependency order (地基 → 上层).

Last updated: 2026-02-01

## Status

### Done (archived in OpenSpec)
- ✅ `update-skills-files-pages` — `/skills` + `/files` pages; default open in new tab
- ✅ `add-skill-versions` — Skills versions store + API + UI
- ✅ `add-skills-hub-governance` — tool adapters, onboarding/import/install/update + UI
- ✅ `add-acceptance-gates` — acceptance state + deterministic helpers + UI progress/report
- ✅ `refactor-web-src-structure` — split `web/src/App.vue` into mid-grain components/composables
- ✅ `add-sandbox-presets` — safe-by-default run/resume presets across workers (UI + API + worker arg mapping)

### In progress
- (none)

## Next (foundation → upper layers)

See `VISION.md` for the “why” and the user-experience driven priorities.

### Foundations (next)
These reduce cognitive load, improve continuity, speed, and safety.
- `add-conversation-id` — CCX-managed stable conversation/thread; decouple from provider `session_id`
- `add-continue-cta` — unify Resume/Rehydrate/Merge guidance into 1 primary “Continue” action
- `add-next-action-engine` — conversation-level state machine + recommended next action (CTA + reason)
- `expand-secretary-level-2-3` — full decision + escalation + audit trail loop
- `add-context-snapshots` — summarize/trim context for rehydrate + long threads (speed + stability)

### Upper layers (queue)
These are valuable, but should follow the foundations above:
- `add-context-and-templates`
- `add-quick-actions`
- `add-file-advanced-capabilities`
- `add-editor-monaco`

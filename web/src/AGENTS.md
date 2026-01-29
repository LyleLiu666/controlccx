# Web Frontend (AI-Friendly Map)

This folder is intentionally organized so both humans and AI agents can quickly locate the right place to modify behavior without “searching the whole app”.

## Entry Points
- `web/src/App.vue`: **composition root** (page layout + wiring). Prefer keeping domain logic out of here.
- `web/src/main.ts`: mounts Vue app.
- `web/src/api.ts`: HTTP client to backend `/api/*` endpoints.
- `web/src/types.ts`: shared frontend types (mirrors backend JSON shapes).

## Structure (preferred)
- `web/src/components/`: mid-grain UI blocks (Modal/Drawer/Panel). Avoid micro-components.
  - Naming: `XxxModal.vue`, `XxxDrawer.vue`, `XxxPanel.vue`
- `web/src/composables/`: state + interactions per domain (Vue composition functions).
  - Naming: `useXxx.ts`
- `web/src/utils/`: pure helpers (no Vue refs, no side effects)
- `web/src/styles/`: shared styles/tokens (migrate incrementally; do not rewrite everything at once)

## Where to Change What (target state)
- Skills UI: `components/SkillsModal.vue` + `composables/useSkills.ts`
- Secretary chat/drawer: `components/SecretaryDrawer.vue` + `composables/useSecretaryChat.ts`
- Live feed: `components/LiveDrawer.vue` + `composables/useLiveFeed.ts`
- Tasks + logs + trace: `composables/useTasks.ts` (and keep API calls in `api.ts`)
- Files (tree/preview/edit): `components/FilesModal.vue` + `composables/useFs.ts`

## Guardrails (important for long-term maintainability)
- Prefer **mid-grain extraction** (natural UI boundaries). Do not explode into dozens of tiny components.
- Keep naming consistent and descriptive. One module should have one obvious home.
- Avoid hidden global coupling: composables should take explicit inputs and return explicit outputs.
- CSS: keep working throughout; extract only when a block stabilizes.

## Verification (required)
Run after every meaningful change:
- `pnpm -C web build`
- `pnpm smoke`

If backend code is touched:
- `go test ./...`

## Manual Smoke Checklist (quick)
- Open Skills modal, filter, page Next/Prev, enable/disable does not break UI
- Open Secretary drawer, long messages readable, input stays visible
- Open Live, logs stream, wrap/pause works
- Files modal: tree loads, preview/edit/save works
- New Run modal: create run, blocked state shows guidance


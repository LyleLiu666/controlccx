## 1. Update OpenSpec Change Docs (this change)
- [x] Ensure delta specs exist under `openspec/changes/fix-session-menu-popup-overlay/specs/` with scenarios
- [x] Run `openspec validate fix-session-menu-popup-overlay --type change --strict --no-interactive`

## 2. Implementation
- [x] Render the session “⋯” actions menu as a teleported overlay (not clipped by panel overflow)
- [x] Close behavior: click-outside, Esc, selecting another session
- [x] Ensure the overlay is positioned near the trigger and clamped to viewport

## 3. Verification
- [x] Run `node --test web/tests/*.test.ts`
- [x] Run `npm -C web run build`

## 4. Close the Loop
- [x] Update `openspec/project.md` timeline (prepend newest)
- [x] Archive: `openspec archive fix-session-menu-popup-overlay --skip-specs --yes`

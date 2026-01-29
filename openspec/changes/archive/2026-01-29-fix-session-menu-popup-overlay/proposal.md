# Change: fix-session-menu-popup-overlay

## Why
The session “⋯” actions popup is currently clipped by the sessions panel scroll container (`overflow`), so users cannot see or
click menu items reliably (especially near the bottom of the list).

This is a cockpit UX regression: the menu is a low-frequency control, but it must always be accessible when needed.

## What Changes
- Render the session row actions menu as an overlay (teleported to `body`) positioned next to the trigger button so it is not
  clipped by panel/container overflow.
- Preserve keyboard/click-outside behavior to close the menu.

## Principles & Metrics
- Improves **Cockpit-first UX** (progressive disclosure that still works).
- Improves **Stuck perception** indirectly (no “I can’t click the menu” friction).

## Impact
- Frontend:
  - `web/src/App.vue`


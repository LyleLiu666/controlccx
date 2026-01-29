## 1. Web UI (Secretary Drawer)
- [x] 1.1 Enlarge Secretary drawer for desktop and near-fullscreen on mobile
- [x] 1.2 Re-layout chat: messages take full height; input pinned bottom
- [x] 1.3 Render chat messages as Markdown (preserve paragraphs/newlines; support tables/code; mermaid where present)
- [x] 1.4 Enter-to-send (Shift+Enter newline; IME composing does not send)

## 2. Verification
- [x] 2.1 Run `openspec validate update-secretary-drawer-ui --strict --no-interactive`
- [x] 2.2 Run `pnpm -C web build` and `pnpm build`
- [x] 2.3 Run `pnpm smoke`

## 1. OpenSpec
- [x] 1.1 Add delta spec (`web-ui`)
- [x] 1.2 Run `openspec validate update-mobile-shell-polish --strict --no-interactive`

## 2. Implementation
- [x] 2.1 Replace key `100vh` usages with `100dvh` (keep fallback) for mobile shell containers
- [x] 2.2 Respect safe-area inset for fixed UI elements on narrow screens
- [x] 2.3 Add reduced-motion CSS overrides

## 3. Verification
- [x] 3.1 Run `go test ./...`
- [x] 3.2 Run `node --test web/tests/*.test.ts`
- [x] 3.3 Run `npm -C web run build`

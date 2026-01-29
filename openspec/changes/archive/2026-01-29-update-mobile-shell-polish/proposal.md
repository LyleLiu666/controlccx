## Why
移动端壳层（Mobile Shell）已经具备基本可用性（抽屉、触控目标、遮罩），但仍存在典型“移动端细节”问题：
1) iOS/移动浏览器地址栏与键盘导致 `100vh` 不稳定，抽屉/面板高度会跳动或被遮挡；
2) safe-area（刘海/底部 Home 指示条）下固定浮层容易贴边遮挡；
3) 无障碍（reduce motion）用户需要减少 hover/位移类动效。

这些属于“驾驶室”体验的地基：不解决会让用户觉得“不稳”“不可信”，与愿景冲突。

## What Changes
- 统一移动端高度计算：关键容器使用 `100dvh`（带 `100vh` fallback），减少键盘/地址栏引发的高度抖动。
- 关键固定浮层（例如 Secretary orb、toast）对齐 safe-area inset，避免被底部区域遮挡。
- 支持 `prefers-reduced-motion: reduce`：禁用位移/过渡类动效，提升可访问性与稳定感。

## Impact
- Affected specs: `web-ui`
- Affected code: `web/src/App.vue`


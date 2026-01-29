# Change: Mobile Shell Baseline

## Why
ControlCCX 当前的移动端体验是“能用但不稳”，缺少系统性的触控/键盘/安全区适配，导致手机上使用成本高。

## What Changes
- 提供移动端“驾驶室”壳层：会话列表默认折叠、主面板全宽、抽屉/浮层统一交互
- 触控目标尺寸与间距规范化（按钮/标签/开关）
- 键盘弹出时保证输入框可见，避免被遮挡

## Impact
- Affected specs: `web-ui`
- Affected code: `web/src/App.vue`（布局与样式）

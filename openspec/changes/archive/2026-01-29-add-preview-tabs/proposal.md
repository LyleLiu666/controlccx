# Change: Preview Tabs (Markdown/Raw/HTML)

## Why
当前仅有 Markdown 渲染 + 高亮，缺少 HTML 与原始视图，多场景预览不足。

## What Changes
- 结果区域新增 Markdown / Raw / HTML 三种预览
- HTML 预览使用安全沙箱显示

## Impact
- Affected specs: `web-ui`
- Affected code: `web/src/App.vue`

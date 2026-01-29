## Why
当前 dashboard 的“驾驶室”体验仍有两处不够聚焦：

1) Sessions 列表为了更紧凑而压缩高度后，workdir/prompt 的呈现被折叠得不够优雅（信息像被“挤没了”）。
2) Session Detail 顶部缺少“当前 run 正在处理的指令/提示”的可见性，用户在多 run 场景下很难一眼确认正在看的/正在跑的是哪条指令。

## What Changes
- Sessions 列表卡片在紧凑布局下仍能稳定展示关键摘要：
  - workdir 以更短更可辨的方式展示（优先显示 pinned workspace name，否则显示短路径）
  - prompt 以单行省略号展示，并在 tooltip 中可查看完整内容
- Session Detail 顶部增加“当前 run 指令（mode + prompt 摘要）”提示，避免用户迷失。

## Impact
- 仅 UI/前端逻辑变更；不改后端 API 与数据结构。
- 兼容现有布局；在窄屏/移动端仍保持单行可读并自动省略。

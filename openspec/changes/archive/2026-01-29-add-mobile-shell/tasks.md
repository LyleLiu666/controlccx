## 1. Spec
- [x] 1.1 定义移动端壳层与布局规则（断点、抽屉、主面板）
- [x] 1.2 定义触控目标最小尺寸与间距
- [x] 1.3 `openspec validate add-mobile-shell --strict --no-interactive`

## 2. Frontend
- [x] 2.1 抽屉与主面板在窄屏下的默认态与切换
- [x] 2.2 统一触控目标尺寸（按钮/标签/开关）
- [x] 2.3 输入框在键盘弹出时保持可见

## 3. Validation
- [x] 3.1 `pnpm -C web build`
- [x] 3.2 `pnpm smoke`

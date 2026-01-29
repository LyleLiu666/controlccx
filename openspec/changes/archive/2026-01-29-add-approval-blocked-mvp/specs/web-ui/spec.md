## ADDED Requirements

### Requirement: Blocked state UX for approval-required runs
当某个 run 进入 `blocked`（因为需要 approval）时，UI MUST 清晰展示原因并提供下一步引导。

#### Scenario: Sessions list shows blocked reason
- **GIVEN** 某个 session 的最新 run 为 `blocked`
- **WHEN** UI 展示 sessions 列表
- **THEN** UI MUST 以醒目的状态展示 `blocked`
- **AND** UI SHOULD 在 hover/title 中展示阻塞原因摘要

#### Scenario: Session detail provides next step hint
- **GIVEN** 当前选中的 run 为 `blocked`
- **WHEN** 用户查看 Session Detail
- **THEN** UI MUST 展示下一步建议（例如切换审批策略/重试/Resume）


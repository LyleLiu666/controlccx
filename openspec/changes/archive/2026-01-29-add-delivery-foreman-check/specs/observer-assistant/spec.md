## ADDED Requirements

### Requirement: Delivery Foreman auto-check
当某个任务 run 进入终态（例如 succeeded/failed/blocked），系统 SHALL 触发一次“交付前哨”流程，由 Observer/Secretary 判断是否为复杂任务并给出后续引导。

#### Scenario: Complex task triggers industrial-grade checklist
- **GIVEN** 一个 run 已进入终态
- **AND** Observer 判断该任务为复杂任务
- **WHEN** 交付前哨流程触发
- **THEN** Observer SHALL 输出工业级交付 checklist（测试、构建、回归、自评、风险点）
- **AND** 输出 SHALL 以可执行的步骤/命令为主（不默认执行）

#### Scenario: Not-finished tasks get resume guidance
- **GIVEN** 一个 run 已进入终态
- **WHEN** Observer 判断用户目标未真正完成
- **THEN** Observer SHALL 提供下一步最小 resume prompt（以 user 要输入的内容形式）
- **AND** 给出需要补齐的关键点列表

#### Scenario: Simple tasks do not spam
- **GIVEN** 一个 run 已进入终态
- **AND** Observer 判断该任务为简单任务
- **WHEN** 交付前哨流程触发
- **THEN** Observer SHALL 用一句话说明“无需工业级交付检查”并结束

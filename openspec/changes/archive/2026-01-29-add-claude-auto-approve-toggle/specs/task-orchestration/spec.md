## ADDED Requirements

### Requirement: Per-run auto-approve execution
The system MUST support a per-run “auto-approve” execution mode for `claude-code` that controls whether `--dangerously-skip-permissions` is passed.

#### Scenario: Default is safe
- **WHEN** a new `claude-code` task is created without auto-approve enabled
- **THEN** the worker invocation MUST NOT include `--dangerously-skip-permissions`

#### Scenario: Auto-approve enabled
- **WHEN** a new `claude-code` task is created with auto-approve enabled
- **THEN** the worker invocation MUST include `--dangerously-skip-permissions`

#### Scenario: Traceability
- **WHEN** a run starts
- **THEN** the system MUST record enough invocation metadata to know whether auto-approve was enabled


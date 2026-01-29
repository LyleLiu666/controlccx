# worker-codex (delta)

## ADDED Requirements

### Requirement: Default Codex sandbox allows workspace writes
When running the Codex worker in non-unsafe mode, the system SHALL invoke `codex exec` with a sandbox mode that allows
writing within the workspace (`workspace-write`).

#### Scenario: Safe mode uses workspace-write sandbox
- **GIVEN** a task uses the Codex worker
- **AND** unsafe automation is disabled (globally and per-run)
- **WHEN** the system builds the Codex command
- **THEN** the command arguments include `--sandbox workspace-write`

### Requirement: Unsafe automation keeps existing bypass behavior
When unsafe automation is enabled (globally or per-run), the system SHALL keep using the explicit bypass flag
`--dangerously-bypass-approvals-and-sandbox`.

#### Scenario: Unsafe mode uses dangerous bypass flag
- **GIVEN** a task uses the Codex worker
- **AND** unsafe automation is enabled (globally or per-run)
- **WHEN** the system builds the Codex command
- **THEN** the command arguments include `--dangerously-bypass-approvals-and-sandbox`


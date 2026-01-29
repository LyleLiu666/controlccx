## ADDED Requirements

### Requirement: Claude Code auto-approve toggle
Web UI MUST provide a per-run “Auto-approve tools” toggle for `claude-code`, with clear safety copy.

#### Scenario: Toggle shown only for claude-code
- **WHEN** the user selects worker `claude-code`
- **THEN** the UI shows an “Auto-approve tools” toggle
- **AND** the UI explains that enabling it skips interactive approvals

#### Scenario: Toggle persisted
- **WHEN** the user changes the toggle value
- **THEN** the UI persists the preference locally (e.g. localStorage)
- **AND** the next New Run defaults to that persisted value

#### Scenario: Toggle applied to resume
- **GIVEN** the user is resuming a session
- **WHEN** the user triggers Resume
- **THEN** the same auto-approve preference is applied to that resumed run


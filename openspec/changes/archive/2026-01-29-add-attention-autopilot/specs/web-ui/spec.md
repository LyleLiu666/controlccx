## ADDED Requirements

### Requirement: Attention Autopilot toggle
The UI SHALL provide a toggle to enable/disable “Attention Autopilot”, persisted across reloads.

#### Scenario: User disables autopilot
- **GIVEN** Autopilot is enabled
- **WHEN** the user disables it in Secretary → Overview
- **THEN** the system SHALL stop any new automatic resume attempts
- **AND** the setting SHALL persist across page reloads

### Requirement: Automatic resume attempt for interrupted sessions
When Autopilot is enabled, the UI SHALL automatically attempt to resume sessions that are `interrupted`, once per session (rate-limited).

#### Scenario: Interrupted session is auto-resumed
- **GIVEN** a session status becomes `interrupted`
- **AND** the session has a valid `session_id` and is not deleted
- **WHEN** Autopilot is enabled
- **THEN** the system SHALL create a resume run with a minimal prompt (e.g. “continue”)
- **AND** the UI SHOULD surface the outcome (started / failed / blocked)

### Requirement: Autopilot MUST not loop endlessly
Autopilot MUST be deduplicated and rate-limited to avoid infinite retries and noisy UI.

#### Scenario: Resume repeatedly fails
- **GIVEN** an interrupted session cannot be resumed (e.g. “No conversation found …”)
- **WHEN** Autopilot attempts a resume
- **THEN** the system SHALL stop further automatic attempts for that session
- **AND** the user SHALL see a clear reason and a suggested next step

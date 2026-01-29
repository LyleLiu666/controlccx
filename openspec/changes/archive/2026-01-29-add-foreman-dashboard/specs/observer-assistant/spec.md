## ADDED Requirements

### Requirement: Secretary can take recovery actions
The system SHALL allow the Secretary agent to trigger recovery actions (like resuming a session) without requiring the user to manually recreate a run.

#### Scenario: Auto resume an interrupted session
- **GIVEN** a run is in a terminal state such as `interrupted` / `blocked` / `failed`
- **AND** the run belongs to a session with a valid `session_id`
- **WHEN** the user asks the Secretary to “continue / resume”
- **THEN** the Secretary SHOULD start a new resume run for that session (same tool + workdir + session_id)
- **AND** the Secretary response SHOULD clearly state what action was taken (new run id / reason if not possible)

#### Scenario: Avoid overlapping runs
- **GIVEN** a session already has a `running` or `queued` run
- **WHEN** the Secretary attempts to start a resume run
- **THEN** the system MUST reject the action to prevent overlapping runs

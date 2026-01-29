## ADDED Requirements

### Requirement: Secretary SHOULD handle “needs attention” without user babysitting
When a run/session needs attention, the Secretary SHALL attempt to continue progress automatically when it is safe and deterministic, only escalating to the user when necessary.

#### Scenario: Interrupted run gets auto-continued
- **GIVEN** a session is marked as needing attention due to `interrupted`
- **WHEN** Autopilot is enabled
- **THEN** the Secretary/system SHALL attempt to resume with a minimal prompt (e.g. “continue”)
- **AND** surface what it did and why

### Requirement: Escalation only when secretary cannot decide
If automatic continuation is unsafe, ambiguous, or impossible, the system SHALL escalate with a concise explanation and the smallest next action required from the user.

#### Scenario: Resume is impossible
- **GIVEN** the system detects “No conversation found …”
- **WHEN** an auto-resume attempt fails
- **THEN** the system SHALL stop auto attempts for that session
- **AND** provide an actionable explanation (e.g. session missing; suggest starting a new run or selecting a different session)

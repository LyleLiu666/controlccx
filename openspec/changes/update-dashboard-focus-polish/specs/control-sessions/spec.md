## MODIFIED Requirements

### Requirement: Session-First Navigation
The system SHALL present sessions (grouped by `session_id`) as the primary navigational object, with a per-session run history, and MUST remain readable under compact layouts.

#### Scenario: Compact session card remains readable
- **GIVEN** the Sessions list is displayed in compact mode
- **WHEN** a session card is rendered
- **THEN** the card SHALL show `session_id` (or task id) and the session status
- **AND** the card SHALL show a compact `workdir` label (pinned workspace name when available, otherwise a short path)
- **AND** the card SHALL show the latest prompt summary in a single line with ellipsis
- **AND** the full `workdir` and prompt SHALL remain accessible via tooltip

#### Scenario: Detail header indicates current run instruction
- **GIVEN** a session is selected
- **WHEN** a run is selected or running
- **THEN** the Session Detail header SHALL display the run mode (`new`/`resume`) and prompt summary in a single line with ellipsis

## ADDED Requirements

### Requirement: Workspace Filter And Pinning
The system SHALL allow users to scope session navigation by an active workspace path and pin frequently used workspaces with optional display names.

#### Scenario: Filter sessions by workspace
- **WHEN** the user selects a workspace path as the active filter
- **THEN** the sessions list SHALL only include sessions whose `workdir` is within that workspace

#### Scenario: Pin a workspace
- **WHEN** the user pins a workspace path
- **THEN** the workspace SHALL appear in a pinned list for fast selection

#### Scenario: Name a pinned workspace
- **WHEN** the user assigns or edits a display name for a pinned workspace
- **THEN** the UI SHALL display that name while preserving the underlying workspace path

### Requirement: Session-First Navigation
The system SHALL present sessions (grouped by `session_id`) as the primary navigational object, with a per-session run history.

#### Scenario: Switch between sessions
- **WHEN** the user selects a session from the sessions list
- **THEN** the UI SHALL display that session’s run history and latest status

#### Scenario: Switch between runs in a session
- **WHEN** the user selects a run within a session
- **THEN** the UI SHALL display logs and metadata for that run

### Requirement: Global Secretary Overview
The system SHALL provide a single “Secretary” overview that observes all sessions and highlights sessions requiring attention.

#### Scenario: Highlight sessions needing attention
- **WHEN** a session is blocked, failed, or has a high score
- **THEN** the Secretary view SHALL list it in a “needs attention” section

#### Scenario: Navigate from secretary to a session
- **WHEN** the user clicks an entry in the Secretary “needs attention” list
- **THEN** the UI SHALL navigate to that session/run context

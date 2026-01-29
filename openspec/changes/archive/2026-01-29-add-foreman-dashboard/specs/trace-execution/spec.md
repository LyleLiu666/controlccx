## ADDED Requirements

### Requirement: Run Invocation Trace
The system SHALL capture and display run invocation metadata including command, arguments, working directory, and injected environment key names (without secret values).

#### Scenario: View invocation metadata
- **WHEN** a run starts and the user opens the run’s trace view
- **THEN** the UI SHALL show `cmd`, `args`, `dir`, and `env_injected_keys` for that run

#### Scenario: Secrets are not exposed
- **WHEN** the system presents invocation metadata
- **THEN** the system MUST NOT display secret values (only env key names)

### Requirement: Log Filtering And Search
The system SHALL allow users to filter logs by stream and search logs by a substring query.

#### Scenario: Filter by stream
- **WHEN** the user disables a log stream (e.g., `stdout`)
- **THEN** the log view SHALL hide entries from that stream

#### Scenario: Search by substring
- **WHEN** the user provides a non-empty search query
- **THEN** the log view SHALL show only log entries whose message contains the query

### Requirement: Log Export
The system SHALL allow users to export run logs into a downloadable text file.

#### Scenario: Download logs
- **WHEN** the user requests a log export for a run
- **THEN** the system SHALL return a text file containing the run logs in a deterministic order

### Requirement: Replay And Resume Shortcuts
The system SHALL support replaying a run and resuming a session using persisted session identifiers when available.

#### Scenario: Replay a run
- **WHEN** the user clicks “Replay run”
- **THEN** the system SHALL create a new run with the same worker type, workdir, and prompt

#### Scenario: Resume a session
- **WHEN** the user clicks “Resume session” and a persisted session ID exists
- **THEN** the system SHALL create a new run in resume mode using that session ID


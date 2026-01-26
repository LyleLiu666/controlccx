## ADDED Requirements

### Requirement: Task lifecycle management
The system MUST manage tasks with a clear lifecycle and expose task creation, listing, inspection, and cancellation.

#### Scenario: Create a task
- **WHEN** a client submits a new task with a worker type and command payload
- **THEN** the server returns a stable task ID and initial status
- **AND** the task becomes visible in task listing immediately

#### Scenario: Cancel a running task
- **WHEN** a client cancels an in-progress task
- **THEN** the task status transitions to `canceled`
- **AND** no further worker output is appended after cancellation completes

### Requirement: Task status model
The server MUST represent each task using a finite set of statuses.

#### Scenario: Successful completion
- **WHEN** a worker finishes with exit code `0`
- **THEN** the task status becomes `succeeded`

#### Scenario: Failed completion
- **WHEN** a worker finishes with a non-zero exit code
- **THEN** the task status becomes `failed`

### Requirement: Durable task persistence
The server MUST persist tasks so that task history is not lost after process restart.

#### Scenario: Tasks survive restart
- **GIVEN** tasks exist in the system
- **WHEN** the server restarts
- **THEN** previously created tasks are still visible via the task listing endpoint

### Requirement: Resume and reattach
The server MUST support “断点接续” by resuming or re-attaching to in-flight tasks when feasible.

#### Scenario: Reattach to an in-flight task
- **GIVEN** a task is still running when the server process restarts
- **WHEN** the server starts again
- **THEN** the task appears as running
- **AND** new output continues to be recorded and streamed

### Requirement: Supported worker types
The system MUST support running both `claude code` and `codex` workers.

#### Scenario: Start a Claude Code worker
- **WHEN** a task is created with worker type `claude-code`
- **THEN** the server starts an interactive worker session for Claude Code

#### Scenario: Claude Code on Windows uses Git Bash
- **GIVEN** the server is running on Windows
- **WHEN** a task is created with worker type `claude-code`
- **THEN** the server launches Claude Code via a Git Bash environment (configurable path)

#### Scenario: Start a Codex worker
- **WHEN** a task is created with worker type `codex`
- **THEN** the server starts a worker session for Codex

#### Scenario: Codex on Windows is best-effort
- **GIVEN** the server is running on Windows
- **WHEN** a task is created with worker type `codex`
- **THEN** the server attempts to start the worker and capture output
- **AND** the task may be marked with a “degraded” compatibility warning if the environment is unstable

### Requirement: Task problem scoring
The server MUST compute a deterministic “problem score” per task to support identifying problematic tasks.

#### Scenario: Blocked tasks rank higher
- **WHEN** a task enters a `blocked` state
- **THEN** its problem score increases compared to a normal `running` task

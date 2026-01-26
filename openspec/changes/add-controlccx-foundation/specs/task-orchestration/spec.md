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

### Requirement: Task problem scoring
The server MUST compute a deterministic “problem score” per task to support identifying problematic tasks.

#### Scenario: Blocked tasks rank higher
- **WHEN** a task enters a `blocked` state
- **THEN** its problem score increases compared to a normal `running` task


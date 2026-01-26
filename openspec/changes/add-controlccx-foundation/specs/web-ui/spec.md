## ADDED Requirements

### Requirement: Task dashboard
The web UI MUST show a list of tasks with real-time status updates and allow inspecting task logs.

#### Scenario: Live status updates
- **WHEN** a task status changes on the server
- **THEN** the UI reflects the new status without manual refresh

#### Scenario: Pick a work directory
- **WHEN** a user starts a new task
- **THEN** the UI allows selecting a working directory using a folder picker UI
- **AND** the selected directory is used as the task workdir

### Requirement: Chat UI
The web UI MUST allow the user to send messages to the observer and see responses in real time.

#### Scenario: Send a message
- **WHEN** a user submits a chat message
- **THEN** the UI displays the user message and the observer response

### Requirement: System info view
The web UI MUST display basic system information for the running server.

#### Scenario: View server system info
- **WHEN** a user opens the system info panel
- **THEN** the UI shows OS and architecture information from the backend

### Requirement: Worker auth settings
The web UI MUST provide an entrypoint to configure worker authentication and surface a clear hint when auth is missing.

#### Scenario: Show missing auth hint
- **GIVEN** the selected worker has no available auth (per backend auth status)
- **WHEN** a user opens the task creation form
- **THEN** the UI shows a clear warning and provides a Settings entry

#### Scenario: Save auth in UI
- **WHEN** a user saves worker auth secrets in the Settings UI
- **THEN** the UI shows the updated auth status from the backend

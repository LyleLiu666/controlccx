## ADDED Requirements

### Requirement: Task dashboard
The web UI MUST show a list of tasks with real-time status updates and allow inspecting task logs.

#### Scenario: Live status updates
- **WHEN** a task status changes on the server
- **THEN** the UI reflects the new status without manual refresh

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


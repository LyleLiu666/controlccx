## ADDED Requirements

### Requirement: Real-time event stream
The server MUST provide a real-time event stream for task state and logs.

#### Scenario: Subscribe to events
- **WHEN** a client connects to the event stream endpoint
- **THEN** the client receives subsequent task and log events without polling

### Requirement: Event envelope
Each event MUST include a type, timestamp, and payload.

#### Scenario: Task update event
- **WHEN** a task status changes
- **THEN** the server emits an event with type `task.updated`
- **AND** the payload contains the task ID and new status


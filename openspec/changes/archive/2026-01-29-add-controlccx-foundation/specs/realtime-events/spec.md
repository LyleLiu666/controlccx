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

### Requirement: Connection robustness
The server MUST keep the event stream usable for long-lived connections.

#### Scenario: Heartbeat
- **GIVEN** a client is connected to the event stream
- **WHEN** no task events occur for a period of time
- **THEN** the server periodically emits a heartbeat event so intermediaries do not silently close the connection


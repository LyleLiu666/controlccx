## ADDED Requirements

### Requirement: Observer chat
The system MUST allow users to chat with an observer that answers using current task and system context.

#### Scenario: Ask for running task count
- **WHEN** a user asks how many tasks are currently executing
- **THEN** the observer responds with a count grounded in the task store

#### Scenario: Ask for most problematic task
- **WHEN** a user asks which task has the most issues
- **THEN** the observer responds with a ranked answer grounded in task problem scoring

### Requirement: Observer system context
The observer MUST have access to basic system information for grounding (OS, architecture, and hostname at minimum).

#### Scenario: System snapshot available
- **WHEN** the observer handles a user message
- **THEN** it can access a recent system snapshot to include in its reasoning if needed


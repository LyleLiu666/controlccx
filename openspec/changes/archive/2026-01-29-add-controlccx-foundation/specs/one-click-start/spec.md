## ADDED Requirements

### Requirement: One-command development startup
Developers MUST be able to start backend + frontend together with a single command on macOS, Linux, and Windows.

#### Scenario: Start dev stack
- **WHEN** a developer runs the documented dev command
- **THEN** both the backend API and the web UI are reachable locally

### Requirement: Production-like startup
Users MUST be able to run a production-like server that serves the web UI and API together.

#### Scenario: Start production-like server
- **WHEN** a user runs the documented start command
- **THEN** a single HTTP origin serves both the UI and API endpoints

### Requirement: Standalone server binary
Users MUST be able to run the backend server without requiring Node.js at runtime (after assets are built).

#### Scenario: Run server binary
- **GIVEN** the web assets have been built
- **WHEN** a user runs the server binary directly
- **THEN** the UI and API are served from the same origin
- **AND** the UI does not require a separate runtime static directory

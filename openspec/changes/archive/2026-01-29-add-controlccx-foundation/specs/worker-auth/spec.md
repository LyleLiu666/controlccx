## ADDED Requirements

### Requirement: Worker auth visibility
The system MUST expose a way to determine whether each supported worker has usable authentication configured.

#### Scenario: Query auth status
- **WHEN** a client requests the auth status endpoint
- **THEN** the server returns whether `claude-code` and `codex` have authentication available
- **AND** the response MUST NOT include raw secret values (only masked hints)

### Requirement: Persisted worker auth secrets
The system MUST allow the user to persist worker authentication secrets locally so that new tasks can start without restarting the server process.

#### Scenario: Save an API key via API
- **WHEN** a client submits an auth update with a worker API key
- **THEN** the server persists the secret under the configured data directory
- **AND** subsequent tasks started for that worker can use the persisted secret

#### Scenario: Environment variables take precedence
- **GIVEN** a worker auth env var is already set in the server process environment
- **WHEN** a task is started
- **THEN** the worker process uses the env-provided value rather than an overridden stored value

### Requirement: Auth API surface
The server MUST provide a minimal local-only API for reading auth status and updating stored secrets.

#### Scenario: Read auth status
- **WHEN** a client requests `GET /api/auth/status`
- **THEN** the server returns a per-worker auth availability status

#### Scenario: Read auth info
- **WHEN** a client requests `GET /api/auth`
- **THEN** the server returns auth status and the local storage path used for persisted secrets

#### Scenario: Update stored auth
- **WHEN** a client requests `POST /api/auth` with an auth patch payload
- **THEN** the server updates stored secrets and returns the updated auth status

#### Scenario: Reject non-local requests
- **WHEN** a non-loopback client attempts to call `/api/auth` endpoints
- **THEN** the server rejects the request

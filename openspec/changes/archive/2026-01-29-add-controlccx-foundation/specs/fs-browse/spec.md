## ADDED Requirements

### Requirement: File system roots
The server MUST expose a list of allowed file system roots for browsing.

#### Scenario: List roots
- **WHEN** a client requests file system roots
- **THEN** the server returns a list of root entries with a display name and path

### Requirement: Directory listing
The server MUST allow listing subdirectories under allowed roots for use by the web UI folder picker.

#### Scenario: List directory entries
- **GIVEN** a directory path under an allowed root
- **WHEN** a client requests a directory listing for that path
- **THEN** the server returns the immediate child directories

#### Scenario: Block paths outside roots
- **GIVEN** a path outside all allowed roots
- **WHEN** a client requests a directory listing for that path
- **THEN** the server rejects the request

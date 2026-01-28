## ADDED Requirements

### Requirement: Machine Skills Discovery
The system SHALL provide a machine-level view of skills by scanning one or more configured skill source roots and exposing
their skill entries as a list with a stable `name`.

#### Scenario: List skills from a source root
- **WHEN** a configured source root contains skill directories
- **THEN** the system lists each directory name as a skill `name`

### Requirement: Link-Based Enablement Per Target
The system SHALL allow enabling a skill for a target tool by creating a filesystem link in the target tool’s skills
directory that points to the skill’s source directory.

#### Scenario: Enable a skill for Claude by symlink
- **WHEN** the user enables skill `skill-creator` for target `claude`
- **THEN** the system creates a symlink at `~/.claude/skills/skill-creator` pointing to the configured source entry

### Requirement: Unlink Without Data Loss
The system SHALL allow disabling a skill for a target tool by removing the corresponding link entry without deleting the
source skill directory.

#### Scenario: Disable a linked skill
- **WHEN** the user disables a linked skill for a target tool
- **THEN** the link entry is removed and the source directory remains unchanged

### Requirement: Broken Link Detection
The system SHALL detect and surface broken link entries in target skills directories.

#### Scenario: Detect broken symlink
- **WHEN** a target skills directory contains a symlink pointing to a missing path
- **THEN** the system reports the entry as `broken`


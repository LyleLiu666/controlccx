# control-sessions (delta)

## MODIFIED Requirements

### Requirement: Session actions menu must be usable near edges
The system SHALL render the session actions menu (the “⋯” menu on a session card) in a way that is not clipped by scroll
containers or panels, so users can always see and click all menu items.

#### Scenario: Menu opens fully even near bottom of list
- **GIVEN** the sessions list is scrollable
- **AND** a session row is near the bottom edge of the visible list
- **WHEN** the user opens the session actions menu
- **THEN** the menu is fully visible and clickable (not clipped by panel/container overflow)

#### Scenario: Menu closes predictably
- **GIVEN** the session actions menu is open
- **WHEN** the user clicks outside the menu OR presses Escape
- **THEN** the menu closes


## MODIFIED Requirements

### Requirement: Mobile shell layout MUST be stable under browser chrome and keyboards
The UI MUST keep core panels/drawers usable on mobile browsers where address bars and on-screen keyboards change viewport height.

#### Scenario: Mobile viewport height changes during input
- **GIVEN** the UI is open on a mobile browser
- **WHEN** the address bar collapses/expands or the on-screen keyboard appears
- **THEN** core drawers/panels SHALL remain within the visible viewport without clipping critical controls
- **AND** the layout SHOULD avoid large jumps caused by `100vh` instability (use `100dvh` where appropriate)

### Requirement: Fixed UI elements MUST respect safe-area insets
The UI MUST position fixed elements (floating buttons, toasts, drawers) so they are not obscured by notches or the home-indicator safe areas.

#### Scenario: Floating button on iPhone home indicator
- **GIVEN** a device with a bottom safe-area inset
- **WHEN** a floating control (e.g. Secretary orb) is shown
- **THEN** it SHALL not overlap the home indicator area

### Requirement: Reduced motion MUST be supported
The UI MUST support `prefers-reduced-motion: reduce` by disabling non-essential animations/transitions.

#### Scenario: User enables reduce motion
- **GIVEN** the user prefers reduced motion
- **WHEN** the UI is rendered
- **THEN** hover/transition/translate effects SHALL be reduced or disabled


# observer-assistant (delta)

## MODIFIED Requirements

### Requirement: Resume automation requires explicit intent
The observer assistant SHALL only trigger automated “resume” behavior when the user expresses explicit intent (a command),
and SHALL NOT trigger resume when the user only mentions “continue/resume/retry” as part of a longer sentence.

#### Scenario: Mentioning “continue” does not trigger resume
- **GIVEN** there is an interrupted session in task history
- **WHEN** the user says “秘书的机制还是有问题，似乎只会回答continue”
- **THEN** the observer does not start a new resume run automatically

#### Scenario: Explicit resume command triggers resume
- **GIVEN** there is an interrupted session in task history
- **WHEN** the user says “继续”
- **THEN** the observer starts a resume run for the most relevant interrupted session


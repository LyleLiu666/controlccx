## ADDED Requirements

### Requirement: Vision MUST be operationalized
The project MUST maintain an operationalized vision that can be used to order roadmap work, write specs, and verify outcomes.

#### Scenario: A contributor needs an objective “definition of closer to vision”
- **WHEN** a contributor opens `openspec/project.md`
- **THEN** they can find a small set of product principles (3–5) and north-star metrics / acceptance criteria that are observable or testable
- **AND** they can use those criteria to decide whether a proposed change is aligned or should be rejected/deprioritized

### Requirement: Roadmap MUST include a dependency-ordered next queue
The project MUST maintain a dependency-ordered “next changes queue” so work can proceed without re-litigating priorities each iteration.

#### Scenario: A contributor starts a new iteration
- **WHEN** a contributor needs to pick the next OpenSpec change to implement
- **THEN** they can consult `openspec/project.md` and find an explicit dependency-ordered queue of the next changes
- **AND** they can start with the earliest item in the queue unless there is a documented reason to reorder

### Requirement: Iteration MUST follow a closed loop
Every iteration MUST follow the fixed loop: vision → roadmap → spec → implement → verify → documentation.

#### Scenario: Finishing an iteration
- **WHEN** a contributor finishes implementation and verification for a change
- **THEN** they update `openspec/project.md` / change docs so the next iteration starts with an accurate vision and roadmap

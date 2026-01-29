## ADDED Requirements

### Requirement: Secretary drawer is readable and chat-friendly
Web UI MUST provide a Secretary drawer that is comfortable for reading long messages and chatting.

#### Scenario: Chat messages use full available height
- **GIVEN** the Secretary drawer is open in Chat view
- **WHEN** the message list is rendered
- **THEN** the messages area SHALL fill the available vertical space and be scrollable
- **AND** the input area SHALL remain visible at the bottom

#### Scenario: Markdown rendering for chat content
- **WHEN** a chat message contains Markdown (paragraphs, lists, tables, code blocks)
- **THEN** the UI SHALL render it as Markdown (not plain text)
- **AND** mermaid blocks SHOULD render consistently with the existing Markdown renderer

#### Scenario: Enter-to-send with IME safety
- **WHEN** the user presses Enter in the chat input
- **THEN** the UI SHALL send the message
- **AND** **WHEN** the user presses Shift+Enter
- **THEN** the UI SHALL insert a newline
- **AND** **WHEN** the user is composing via IME (e.g. Chinese input)
- **THEN** Enter MUST NOT send the message

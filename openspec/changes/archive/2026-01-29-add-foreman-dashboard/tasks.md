## 1. Backend
- [x] 1.1 Add schema for run invocation records (SQLite migration)
- [x] 1.2 Persist invocation metadata at run start (cmd/args/dir/env key names)
- [x] 1.3 Add API to fetch invocation metadata per task/run
- [x] 1.4 Extend logs API to support stream filtering + substring query
- [x] 1.5 Add log export endpoint (downloadable text)

## 2. Frontend
- [x] 2.1 Add “Trace” panel for a run (invocation + exit status)
- [x] 2.2 Add log stream toggles + search input in log view
- [x] 2.3 Add “Copy logs” + “Download logs” actions
- [x] 2.4 Add “Replay run” shortcut and “Resume session” shortcut (guard rails + confirmation)
- [x] 2.5 Improve discoverability for session switching and workspace filtering
- [x] 2.6 Support optional display names for pinned workspaces (rename)

## 3. Secretary
- [x] 3.1 Add attention queue actions (navigate, resume, cancel)
- [x] 3.2 Add briefing scope toggle (All vs current workspace)

## 4. Tests
- [x] 4.1 Unit tests for invocation persistence and redaction rules
- [x] 4.2 API tests for log query and export
- [x] 4.3 UI smoke test for trace/log panels (minimal)

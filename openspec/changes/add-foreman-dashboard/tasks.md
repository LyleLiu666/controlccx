## 1. Backend
- [ ] 1.1 Add schema for run invocation records (SQLite migration)
- [ ] 1.2 Persist invocation metadata at run start (cmd/args/dir/env key names)
- [ ] 1.3 Add API to fetch invocation metadata per task/run
- [ ] 1.4 Extend logs API to support stream filtering + substring query
- [ ] 1.5 Add log export endpoint (downloadable text)

## 2. Frontend
- [ ] 2.1 Add “Trace” panel for a run (invocation + exit status)
- [ ] 2.2 Add log stream toggles + search input in log view
- [ ] 2.3 Add “Copy logs” + “Download logs” actions
- [ ] 2.4 Add “Replay run” shortcut and “Resume session” shortcut (guard rails + confirmation)
- [ ] 2.5 Improve discoverability for session switching and workspace filtering
- [ ] 2.6 Support optional display names for pinned workspaces (rename)

## 3. Secretary
- [ ] 3.1 Add attention queue actions (navigate, resume, cancel)
- [ ] 3.2 Add briefing scope toggle (All vs current workspace)

## 4. Tests
- [ ] 4.1 Unit tests for invocation persistence and redaction rules
- [ ] 4.2 API tests for log query and export
- [ ] 4.3 UI smoke test for trace/log panels (minimal)

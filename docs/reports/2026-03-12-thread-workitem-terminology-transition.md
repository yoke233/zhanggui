# Thread / WorkItem Terminology Transition — Regression Report

**Date:** 2026-03-12
**Branch:** `codex/thread-workitem-terminology-transition`
**Base:** `main` (`7ae041b1`)

## Commits (14 total)

| Hash | Summary |
|------|---------|
| `ed1c6f42` | docs(domain): define thread workitem naming transition |
| `8a6c251b` | feat(thread): add core thread storage and http routes |
| `dd68013a` | feat(ws): add independent thread websocket protocol |
| `cc7e4950` | feat(thread): add ThreadMessage/ThreadParticipant models, API, and UI pages |
| `b67c0048` | fix(api): add thread methods to ApiClient interface for tsc -b compat |
| `eb2da1e7` | docs(domain): define thread work item linking model |
| `732acd23` | remove old files |
| `d7eb1904` | feat(store): persist thread work item links with explicit cleanup |
| `b53bf49f` | feat(api): expose thread work item linking endpoints |
| `84f55716` | feat(ui): surface thread and work item links in detail pages |
| `1d8e4638` | docs(thread): define thread agent runtime model |
| `b143f9bc` | feat(thread): add thread agent runtime support |
| `bed3aedf` | feat(ui): promote threads and work items as primary entry points |
| `ca247993` | feat(thread): add create/link work item from thread flow |

## Verification Results

### Backend Tests

| Package | Result | Duration |
|---------|--------|----------|
| `internal/core` | no test files | — |
| `internal/adapters/store/sqlite` | PASS | 2.8s |
| `internal/adapters/http` | PASS | 10.7s |
| `internal/adapters/http/server` | PASS | 0.03s |

### Frontend Tests

| Suite | Result |
|-------|--------|
| `src/lib/apiClient.test.ts` (15 tests) | PASS |
| `src/lib/scm.test.ts` (6 tests) | PASS |
| `src/lib/wsClient.test.ts` (1 test) | PASS |
| `src/lib/skills.test.ts` (4 tests) | PASS |
| `src/stores/settingsStore.test.ts` (4 tests) | PASS |
| `src/components/SandboxSupportPanel.test.tsx` (1 test) | PASS |
| Total non-legacy | **31 passed, 0 failed** |

Legacy archive failures (pre-existing, unrelated):
- `archive-src/legacy-ui/src/App.test.tsx` — missing import `./App`
- `archive-src/legacy-ui/src/components/SettingsPanel.test.tsx` — missing import `../stores/settingsStore`

### Frontend Build

| Check | Result |
|-------|--------|
| `tsc --noEmit` (typecheck) | PASS |
| `tsc -b && vite build` (production build) | PASS |
| Output size | 679.76 kB (gzip: 199.64 kB) |

## New Test Coverage

### Store layer (`thread_test.go`)

- `TestThreadCRUD` — create, get, list, update, delete thread
- `TestThreadMessageCRUD` — create/list messages, thread_id binding
- `TestThreadParticipantCRUD` — add/list/remove participants
- `TestThreadWorkItemLinkCRUD` — create link, list by thread/work-item, duplicate constraint, delete
- `TestThreadWorkItemLinkCleanup` — cleanup by thread, cleanup by work item
- `TestThreadAgentSessionCRUD` — create, get, list, update status, delete, duplicate profile constraint

### HTTP layer (`thread_test.go`)

- `TestThreadCRUD` — full CRUD lifecycle via HTTP
- `TestThreadCreateMissingTitle` — 400 on empty title
- `TestThreadGetNotFound` — 404 on missing thread
- `TestThreadMessageCRUD` — message create/list via HTTP, 404 on non-existent thread
- `TestThreadParticipantCRUD` — add/list/remove participants via HTTP
- `TestThreadWorkItemLinkCRUD` — create/list/reverse-list/delete links via HTTP
- `TestThreadAgentSessionCRUD` — invite/list/remove agents via HTTP
- `TestThreadCreateWorkItem` — create work item from thread, verify auto-created primary link
- `TestThreadAndIssueRoutesIndependent` — /issues and /threads coexist

### API client (`apiClient.test.ts`)

- `createThreadWorkItemLink` — POST link request
- `listWorkItemsByThread` — GET links by thread
- `listThreadsByWorkItem` — GET reverse links by work item

## Conclusion

All backend and frontend tests pass. No regressions detected. The Thread domain layer, Thread-WorkItem linking, Thread Agent runtime, and UI route promotion are all verified and working correctly.

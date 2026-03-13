# Real ACP JSON Traces

这份文档记录真实 ACP 代理在一次最小会话中的完整 JSON-RPC 收发留档。

重跑命令：

```powershell
go test -tags probe ./cmd/acp-probe -run TestCaptureRealACPJSONTranscripts -v -timeout 600s
```

说明：

- `send` 表示 ai-workflow 发给 ACP agent 的 JSON。
- `recv` 表示 ACP agent 回给 ai-workflow 的 JSON，包含 response、notification，以及 agent 反向发起的 request。
- 完整原文保存在 `docs/reports/artifacts/acp-real-traces/*.json`。

## Summary

| Agent | Status | Session | Trace Count | Event Count | Artifact |
| --- | --- | --- | ---: | ---: | --- |
| `claude-acp` | `passed` | `57f4e1f6-3a93-4ca5-b291-345ab2356a07` | 12 | 6 | [`claude-acp.json`](./artifacts/acp-real-traces/claude-acp.json) |
| `codex-acp` | `passed` | `019ce509-e188-7a00-9abf-38fb4918db3d` | 12 | 6 | [`codex-acp.json`](./artifacts/acp-real-traces/codex-acp.json) |

## claude-acp

- Captured At: `2026-03-13T02:32:47Z`
- Status: `passed`
- Launch: `npx -y @zed-industries/claude-agent-acp`
- Env Keys: `CLAUDECODE`
- Supports SSE MCP: `true`
- Prompt: `Reply with exactly: ACP_REAL_TRACE_OK`
- Stop Reason: `end_turn`
- Result Text: `ACP_REAL_TRACE_OK`

### Trace Flow

- `#1` `send` `2026-03-13T02:32:47.5346599Z` `request initialize id="1"`
- `#2` `recv` `2026-03-13T02:32:49.2808327Z` `response id="1"`
- `#3` `send` `2026-03-13T02:32:49.2808327Z` `request session/new id="2"`
- `#4` `recv` `2026-03-13T02:32:54.1146053Z` `response id="2"`
- `#5` `send` `2026-03-13T02:32:54.11613Z` `request session/prompt id="3"`
- `#6` `recv` `2026-03-13T02:32:54.11613Z` `notification session/update`
- `#7` `recv` `2026-03-13T02:32:55.7139761Z` `notification session/update`
- `#8` `recv` `2026-03-13T02:32:55.7144793Z` `notification session/update`
- `#9` `recv` `2026-03-13T02:32:55.9001438Z` `notification session/update`
- `#10` `recv` `2026-03-13T02:32:55.9048019Z` `notification session/update`
- `#11` `recv` `2026-03-13T02:32:55.9317473Z` `notification session/update`
- `#12` `recv` `2026-03-13T02:32:55.9317473Z` `response id="3"`

## codex-acp

- Captured At: `2026-03-13T02:32:34Z`
- Status: `passed`
- Launch: `npx -y @zed-industries/codex-acp`
- Supports SSE MCP: `false`
- Prompt: `Reply with exactly: ACP_REAL_TRACE_OK`
- Stop Reason: `end_turn`
- Result Text: `ACP_REAL_TRACE_OK`

### Trace Flow

- `#1` `send` `2026-03-13T02:32:34.4601096Z` `request initialize id="1"`
- `#2` `recv` `2026-03-13T02:32:35.9154879Z` `response id="1"`
- `#3` `send` `2026-03-13T02:32:35.9154879Z` `request session/new id="2"`
- `#4` `recv` `2026-03-13T02:32:35.9979804Z` `response id="2"`
- `#5` `send` `2026-03-13T02:32:35.9979804Z` `request session/prompt id="3"`
- `#6` `recv` `2026-03-13T02:32:36.0011646Z` `notification session/update`
- `#7` `recv` `2026-03-13T02:32:47.2640975Z` `notification session/update`
- `#8` `recv` `2026-03-13T02:32:47.2829291Z` `notification session/update`
- `#9` `recv` `2026-03-13T02:32:47.290963Z` `notification session/update`
- `#10` `recv` `2026-03-13T02:32:47.3109239Z` `notification session/update`
- `#11` `recv` `2026-03-13T02:32:47.5180237Z` `notification session/update`
- `#12` `recv` `2026-03-13T02:32:47.5190316Z` `response id="3"`


package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

type contextScopedInput struct {
	Role      string `json:"role,omitempty" jsonschema:"Optional role name used for scope checks (team_leader/reviewer/decomposer/worker/aggregator)"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"Optional project id used for scope checks"`
	IssueID   string `json:"issue_id,omitempty" jsonschema:"Optional issue id used for scope checks"`
}

type ContextReadInput struct {
	contextScopedInput
	URI string `json:"uri" jsonschema:"viking:// URI to read"`
}

type ContextReadOutput struct {
	URI      string `json:"uri"`
	Encoding string `json:"encoding"` // "utf-8" | "base64"
	Content  string `json:"content"`
}

type ContextListInput struct {
	contextScopedInput
	URI string `json:"uri" jsonschema:"viking:// directory URI to list"`
}

type ContextListOutput struct {
	Entries []core.ContextEntry `json:"entries"`
}

type ContextAbstractInput struct {
	contextScopedInput
	URI string `json:"uri" jsonschema:"viking:// URI to abstract"`
}

type ContextAbstractOutput struct {
	URI      string `json:"uri"`
	Abstract string `json:"abstract"`
}

type ContextOverviewInput struct {
	contextScopedInput
	URI string `json:"uri" jsonschema:"viking:// URI to overview"`
}

type ContextOverviewOutput struct {
	URI      string `json:"uri"`
	Overview string `json:"overview"`
}

type ContextFindInput struct {
	contextScopedInput
	Query     string `json:"query" jsonschema:"Semantic search query"`
	TargetURI string `json:"target_uri,omitempty" jsonschema:"Optional target URI scope (prefix)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results to return"`
}

type ContextFindOutput struct {
	Results []core.ContextResult `json:"results"`
}

type ContextSearchInput struct {
	contextScopedInput
	Query     string `json:"query" jsonschema:"Contextual search query"`
	SessionID string `json:"session_id" jsonschema:"OpenViking/Context session id"`
	TargetURI string `json:"target_uri,omitempty" jsonschema:"Optional target URI scope (prefix)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results to return"`
}

type ContextSearchOutput struct {
	Results []core.ContextResult `json:"results"`
}

type ContextWriteInput struct {
	contextScopedInput
	URI      string `json:"uri" jsonschema:"viking:// URI to write"`
	Encoding string `json:"encoding,omitempty" jsonschema:"utf-8 (default) or base64"`
	Content  string `json:"content" jsonschema:"Content to write"`
}

func registerContextTools(server *mcp.Server, store core.ContextStore) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_read",
		Description: "Read a viking:// context entry (returns utf-8 text or base64 bytes)",
	}, contextReadHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_list",
		Description: "List entries under a viking:// context directory URI",
	}, contextListHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_abstract",
		Description: "Get L0 abstract for a viking:// URI",
	}, contextAbstractHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_overview",
		Description: "Get L1 overview for a viking:// URI",
	}, contextOverviewHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_find",
		Description: "Run semantic find over context store",
	}, contextFindHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_search",
		Description: "Run contextual search over context store with session id",
	}, contextSearchHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_write",
		Description: "Write a viking:// context entry (requires write scope)",
	}, contextWriteHandler(store))
}

func contextReadHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextReadInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextReadInput) (*mcp.CallToolResult, any, error) {
		uri := strings.TrimSpace(in.URI)
		if uri == "" {
			return errorResult("uri is required")
		}
		if err := ensureContextReadable(in.contextScopedInput, uri); err != nil {
			return errorResult(err.Error())
		}
		data, err := store.Read(ctx, uri)
		if err != nil {
			return errorResult(fmt.Sprintf("context_read failed: %v", err))
		}
		out := ContextReadOutput{URI: uri, Encoding: "utf-8", Content: string(data)}
		if !utf8.Valid(data) {
			out.Encoding = "base64"
			out.Content = base64.StdEncoding.EncodeToString(data)
		}
		return jsonResult(out)
	}
}

func contextListHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextListInput) (*mcp.CallToolResult, any, error) {
		uri := strings.TrimSpace(in.URI)
		if uri == "" {
			return errorResult("uri is required")
		}
		if err := ensureContextReadable(in.contextScopedInput, uri); err != nil {
			return errorResult(err.Error())
		}
		entries, err := store.List(ctx, uri)
		if err != nil {
			return errorResult(fmt.Sprintf("context_list failed: %v", err))
		}
		return jsonResult(ContextListOutput{Entries: entries})
	}
}

func contextAbstractHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextAbstractInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextAbstractInput) (*mcp.CallToolResult, any, error) {
		uri := strings.TrimSpace(in.URI)
		if uri == "" {
			return errorResult("uri is required")
		}
		if err := ensureContextReadable(in.contextScopedInput, uri); err != nil {
			return errorResult(err.Error())
		}
		abs, err := store.Abstract(ctx, uri)
		if err != nil {
			return errorResult(fmt.Sprintf("context_abstract failed: %v", err))
		}
		return jsonResult(ContextAbstractOutput{URI: uri, Abstract: abs})
	}
}

func contextOverviewHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextOverviewInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextOverviewInput) (*mcp.CallToolResult, any, error) {
		uri := strings.TrimSpace(in.URI)
		if uri == "" {
			return errorResult("uri is required")
		}
		if err := ensureContextReadable(in.contextScopedInput, uri); err != nil {
			return errorResult(err.Error())
		}
		ov, err := store.Overview(ctx, uri)
		if err != nil {
			return errorResult(fmt.Sprintf("context_overview failed: %v", err))
		}
		return jsonResult(ContextOverviewOutput{URI: uri, Overview: ov})
	}
}

func contextFindHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextFindInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextFindInput) (*mcp.CallToolResult, any, error) {
		q := strings.TrimSpace(in.Query)
		if q == "" {
			return errorResult("query is required")
		}
		target := strings.TrimSpace(in.TargetURI)
		if target != "" {
			if err := ensureContextReadable(in.contextScopedInput, target); err != nil {
				return errorResult(err.Error())
			}
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		results, err := store.Find(ctx, q, core.FindOpts{TargetURI: target, Limit: limit})
		if err != nil {
			return errorResult(fmt.Sprintf("context_find failed: %v", err))
		}
		return jsonResult(ContextFindOutput{Results: results})
	}
}

func contextSearchHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextSearchInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextSearchInput) (*mcp.CallToolResult, any, error) {
		q := strings.TrimSpace(in.Query)
		if q == "" {
			return errorResult("query is required")
		}
		sid := strings.TrimSpace(in.SessionID)
		if sid == "" {
			return errorResult("session_id is required")
		}
		target := strings.TrimSpace(in.TargetURI)
		if target != "" {
			if err := ensureContextReadable(in.contextScopedInput, target); err != nil {
				return errorResult(err.Error())
			}
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		results, err := store.Search(ctx, q, sid, core.SearchOpts{TargetURI: target, Limit: limit})
		if err != nil {
			return errorResult(fmt.Sprintf("context_search failed: %v", err))
		}
		return jsonResult(ContextSearchOutput{Results: results})
	}
}

func contextWriteHandler(store core.ContextStore) func(context.Context, *mcp.CallToolRequest, ContextWriteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ContextWriteInput) (*mcp.CallToolResult, any, error) {
		uri := strings.TrimSpace(in.URI)
		if uri == "" {
			return errorResult("uri is required")
		}
		if err := ensureContextWritable(in.contextScopedInput, uri); err != nil {
			return errorResult(err.Error())
		}
		encoding := strings.ToLower(strings.TrimSpace(in.Encoding))
		if encoding == "" {
			encoding = "utf-8"
		}
		var data []byte
		switch encoding {
		case "utf-8", "utf8":
			data = []byte(in.Content)
		case "base64":
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.Content))
			if err != nil {
				return errorResult(fmt.Sprintf("invalid base64 content: %v", err))
			}
			data = decoded
		default:
			return errorResult("encoding must be utf-8 or base64")
		}

		if err := store.Write(ctx, uri, data); err != nil {
			return errorResult(fmt.Sprintf("context_write failed: %v", err))
		}
		return jsonResult(map[string]any{"status": "ok", "uri": uri})
	}
}

func ensureContextReadable(scopeIn contextScopedInput, uri string) error {
	trimmed := strings.TrimSpace(uri)
	if !strings.HasPrefix(trimmed, "viking://") {
		return fmt.Errorf("uri must start with viking://")
	}

	// Default: allow read-only access under resources/ when scope is unknown.
	role := strings.TrimSpace(scopeIn.Role)
	projectID := strings.TrimSpace(scopeIn.ProjectID)
	issueID := strings.TrimSpace(scopeIn.IssueID)
	if role == "" || projectID == "" {
		if strings.HasPrefix(trimmed, "viking://resources/") {
			return nil
		}
		return fmt.Errorf("context read denied: missing scope (role/project_id) and uri is outside viking://resources/")
	}

	scope := ResolveContextScope(role, projectID, issueID)
	for _, prefix := range scope.ReadPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return nil
		}
	}
	return fmt.Errorf("context read denied: uri not in allowed prefixes for role=%s", role)
}

func ensureContextWritable(scopeIn contextScopedInput, uri string) error {
	trimmed := strings.TrimSpace(uri)
	if !strings.HasPrefix(trimmed, "viking://") {
		return fmt.Errorf("uri must start with viking://")
	}
	role := strings.TrimSpace(scopeIn.Role)
	projectID := strings.TrimSpace(scopeIn.ProjectID)
	issueID := strings.TrimSpace(scopeIn.IssueID)
	if role == "" || projectID == "" {
		return fmt.Errorf("context write denied: missing scope (role/project_id)")
	}
	scope := ResolveContextScope(role, projectID, issueID)
	for _, prefix := range scope.WritePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return nil
		}
	}
	return fmt.Errorf("context write denied: uri not in allowed prefixes for role=%s", role)
}

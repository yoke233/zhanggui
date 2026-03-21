package api

import (
	"fmt"
	"net/http"
	"testing"
)

func TestAPI_LegacyCronIssuesRouteRemoved(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := get(ts, "/cron/issues")
	if err != nil {
		t.Fatalf("get legacy cron issues route: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for removed /cron/issues route, got %d", resp.StatusCode)
	}
}

func TestAPI_CreateWorkItemFromTemplateRoute(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/templates", map[string]any{
		"name": "template-a",
		"actions": []map[string]any{
			{"name": "implement", "type": "exec"},
		},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating template, got %d", resp.StatusCode)
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(resp, &created); err != nil {
		t.Fatalf("decode template: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/templates/%d/create-issue", created.ID), map[string]any{"title": "legacy"})
	if err != nil {
		t.Fatalf("post removed create-issue route: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for removed /templates/{id}/create-issue route, got %d", resp.StatusCode)
	}

	resp, err = post(ts, fmt.Sprintf("/templates/%d/create-work-item", created.ID), map[string]any{"title": "from-template"})
	if err != nil {
		t.Fatalf("create work item from template: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating work item from template, got %d", resp.StatusCode)
	}
}

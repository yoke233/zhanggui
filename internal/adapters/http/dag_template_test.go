package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestAPI_CreateDAGTemplateRejectsDuplicateActionNames(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/templates", map[string]any{
		"name": "dup-template",
		"actions": []map[string]any{
			{"name": "implement", "type": "exec"},
			{"name": "implement", "type": "gate"},
		},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 creating template with duplicate action names, got %d", resp.StatusCode)
	}
}

func TestAPI_SaveWorkItemAsTemplateRejectsDuplicateActionNames(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/work-items", map[string]any{
		"title": "dup-actions",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	workItem := decode[core.WorkItem](t, resp)

	resp, err = post(ts, fmt.Sprintf("/work-items/%d/actions", workItem.ID), map[string]any{
		"name": "implement", "type": "exec",
	})
	if err != nil {
		t.Fatalf("create first action: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating first action, got %d", resp.StatusCode)
	}

	resp, err = post(ts, fmt.Sprintf("/work-items/%d/actions", workItem.ID), map[string]any{
		"name": "implement", "type": "gate",
	})
	if err != nil {
		t.Fatalf("create second action: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating second action, got %d", resp.StatusCode)
	}

	resp, err = post(ts, fmt.Sprintf("/work-items/%d/save-as-template", workItem.ID), map[string]any{
		"name": "dup-save",
	})
	if err != nil {
		t.Fatalf("save template: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 saving template with duplicate action names, got %d", resp.StatusCode)
	}
}

func TestAPI_CreateWorkItemFromTemplateRejectsStoredDuplicateActionNames(t *testing.T) {
	h, ts := setupAPI(t)

	tmpl := &core.DAGTemplate{
		Name: "bad-template",
		Actions: []core.DAGTemplateAction{
			{Name: "implement", Type: "exec"},
			{Name: "implement", Type: "gate"},
		},
	}
	id, err := h.store.CreateDAGTemplate(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("seed invalid template: %v", err)
	}

	resp, err := post(ts, fmt.Sprintf("/templates/%d/create-work-item", id), map[string]any{
		"title": "from-bad-template",
	})
	if err != nil {
		t.Fatalf("create work item from template: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 materializing template with duplicate action names, got %d", resp.StatusCode)
	}
}

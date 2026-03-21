package api

import (
	"fmt"
	"net/http"
	"testing"
)

func TestAPI_ListCronWorkItemsViaWorkItemAlias(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/work-items", map[string]any{
		"title":    "nightly template",
		"priority": "medium",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(resp, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/work-items/%d/cron", created.ID), map[string]any{
		"schedule":      "0 2 * * *",
		"max_instances": 2,
	})
	if err != nil {
		t.Fatalf("setup cron: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 setup cron, got %d", resp.StatusCode)
	}

	resp, err = get(ts, "/work-items/cron")
	if err != nil {
		t.Fatalf("list cron alias: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing cron via /work-items/cron, got %d", resp.StatusCode)
	}

	var listed []struct {
		WorkItemID int64 `json:"work_item_id"`
		Enabled    bool  `json:"enabled"`
	}
	if err := decodeJSON(resp, &listed); err != nil {
		t.Fatalf("decode cron list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 cron item, got %d", len(listed))
	}
	if listed[0].WorkItemID != created.ID {
		t.Fatalf("expected work_item_id=%d, got %d", created.ID, listed[0].WorkItemID)
	}
	if !listed[0].Enabled {
		t.Fatal("expected cron item to be enabled")
	}
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestRunToDeliverableResponseIncludesNormalizedArtifact(t *testing.T) {
	run := &core.Run{
		ID:             42,
		ActionID:       7,
		WorkItemID:     9,
		ResultMarkdown: "# Review",
		ResultMetadata: map[string]any{
			core.ResultMetaArtifactNamespace: "gstack",
			core.ResultMetaArtifactType:      "review_report",
			core.ResultMetaArtifactFormat:    "markdown",
			core.ResultMetaArtifactRelPath:   ".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md",
			core.ResultMetaArtifactTitle:     "Login Flow Review",
			core.ResultMetaProducerSkill:     "gstack-review",
			core.ResultMetaProducerKind:      "skill",
			core.ResultMetaSummary:           "Found two correctness issues.",
			"existing":                       "value",
		},
		CreatedAt: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC),
	}

	resp := runToDeliverableResponse(run, nil)

	artifact, ok := resp["artifact"].(map[string]any)
	if !ok {
		t.Fatalf("expected normalized artifact block, got %+v", resp["artifact"])
	}
	if got := artifact["namespace"]; got != "gstack" {
		t.Fatalf("artifact namespace = %v", got)
	}
	if got := artifact["type"]; got != "review_report" {
		t.Fatalf("artifact type = %v", got)
	}
	if got := artifact["format"]; got != "markdown" {
		t.Fatalf("artifact format = %v", got)
	}
	if got := artifact["relpath"]; got != ".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md" {
		t.Fatalf("artifact relpath = %v", got)
	}
	if got := artifact["title"]; got != "Login Flow Review" {
		t.Fatalf("artifact title = %v", got)
	}
	if got := artifact["producer_skill"]; got != "gstack-review" {
		t.Fatalf("artifact producer skill = %v", got)
	}
	if got := artifact["producer_kind"]; got != "skill" {
		t.Fatalf("artifact producer kind = %v", got)
	}
	if got := artifact["summary"]; got != "Found two correctness issues." {
		t.Fatalf("artifact summary = %v", got)
	}

	metadata, ok := resp["metadata"].(map[string]any)
	if !ok || metadata["existing"] != "value" {
		t.Fatalf("expected original metadata to remain intact, got %+v", resp["metadata"])
	}
}

func TestRunToDeliverableResponseOmitsArtifactWhenContractMissing(t *testing.T) {
	run := &core.Run{
		ID:             1,
		ActionID:       2,
		WorkItemID:     3,
		ResultMarkdown: "plain output",
		ResultMetadata: map[string]any{"existing": "value"},
		CreatedAt:      time.Now().UTC(),
	}

	resp := runToDeliverableResponse(run, nil)
	if _, exists := resp["artifact"]; exists {
		t.Fatalf("expected no normalized artifact block, got %+v", resp["artifact"])
	}
}

func TestArtifactRoutesExposeMetadataOnlyResults(t *testing.T) {
	env := setupIntegration(t, nil)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{Title: "artifact-run", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	stepID, err := env.store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "review", Type: core.ActionExec, Status: core.ActionDone})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := env.store.CreateRun(ctx, &core.Run{ActionID: stepID, WorkItemID: workItemID, Status: core.RunSucceeded, Attempt: 1})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	run, err := env.store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	run.ResultMetadata = map[string]any{
		core.ResultMetaArtifactNamespace: "gstack",
		core.ResultMetaArtifactType:      "review_report",
		core.ResultMetaArtifactFormat:    "markdown",
		core.ResultMetaArtifactRelPath:   ".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md",
		core.ResultMetaArtifactTitle:     "Login Flow Review",
	}
	if err := env.store.UpdateRun(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/artifacts/%d", env.server.URL, runID))
	if err != nil {
		t.Fatalf("get artifact by id: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /artifacts/{id} status = %d", resp.StatusCode)
	}
	var single map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&single); err != nil {
		t.Fatalf("decode single artifact: %v", err)
	}
	if _, ok := single["artifact"].(map[string]any); !ok {
		t.Fatalf("expected normalized artifact in single response, got %+v", single)
	}

	resp, err = http.Get(fmt.Sprintf("%s/steps/%d/artifact/latest", env.server.URL, stepID))
	if err != nil {
		t.Fatalf("get latest artifact: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /steps/{stepID}/artifact/latest status = %d", resp.StatusCode)
	}
	var latest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		t.Fatalf("decode latest artifact: %v", err)
	}
	if got := latest["run_id"]; got != float64(runID) {
		t.Fatalf("latest run_id = %v, want %d", got, runID)
	}

	resp, err = http.Get(fmt.Sprintf("%s/runs/%d/artifacts", env.server.URL, runID))
	if err != nil {
		t.Fatalf("list run artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /runs/{runID}/artifacts status = %d", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode run artifacts: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one artifact, got %d", len(items))
	}
}

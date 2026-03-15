package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func postMultipart(ts *httptest.Server, path, fieldName, fileName, contentType string, content []byte) (*http.Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return http.DefaultClient.Do(req)
}

func TestAPI_ResourceSpaceCRUD(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	projectID, err := h.store.CreateProject(ctx, &core.Project{Name: "resource-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	resp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", projectID), map[string]any{
		"kind":     "local_fs",
		"root_uri": "D:/workspace/demo",
		"role":     "primary",
		"label":    "repo",
		"config":   map[string]any{"branch": "main"},
	})
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating space, got %d", resp.StatusCode)
	}
	var space core.ResourceSpace
	if err := decodeJSON(resp, &space); err != nil {
		t.Fatalf("decode space: %v", err)
	}
	if space.ID == 0 || space.Role != "primary" {
		t.Fatalf("unexpected created space: %+v", space)
	}

	resp, err = get(ts, fmt.Sprintf("/projects/%d/spaces", projectID))
	if err != nil {
		t.Fatalf("list spaces: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing spaces, got %d", resp.StatusCode)
	}
	var spaces []*core.ResourceSpace
	if err := decodeJSON(resp, &spaces); err != nil {
		t.Fatalf("decode spaces: %v", err)
	}
	if len(spaces) != 1 || spaces[0].ID != space.ID {
		t.Fatalf("unexpected spaces list: %+v", spaces)
	}

	resp, err = put(ts, fmt.Sprintf("/spaces/%d", space.ID), map[string]any{
		"root_uri": "D:/workspace/updated",
		"role":     "reference",
		"label":    "repo-updated",
		"config":   map[string]any{"branch": "develop"},
	})
	if err != nil {
		t.Fatalf("update space: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 updating space, got %d", resp.StatusCode)
	}
	var updated core.ResourceSpace
	if err := decodeJSON(resp, &updated); err != nil {
		t.Fatalf("decode updated space: %v", err)
	}
	if updated.RootURI != "D:/workspace/updated" || updated.Role != "reference" {
		t.Fatalf("unexpected updated space: %+v", updated)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/spaces/%d", space.ID), nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete space: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 deleting space, got %d", resp.StatusCode)
	}

	if _, err := h.store.GetResourceSpace(ctx, space.ID); err != core.ErrNotFound {
		t.Fatalf("expected resource space deleted, got %v", err)
	}
}

func TestAPI_ActionIODeclRoutesAndValidation(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	projectID, err := h.store.CreateProject(ctx, &core.Project{Name: "io-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{ProjectID: &projectID, Title: "item", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := h.store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "impl", Type: core.ActionExec, Status: core.ActionPending})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	spaceID, err := h.store.CreateResourceSpace(ctx, &core.ResourceSpace{ProjectID: projectID, Kind: core.ResourceKindLocalFS, RootURI: t.TempDir()})
	if err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	resourcePath := filepath.Join(t.TempDir(), "brief.txt")
	if err := os.WriteFile(resourcePath, []byte("brief"), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}
	resourceID, err := h.store.CreateResource(ctx, &core.Resource{ProjectID: projectID, StorageKind: "local", URI: resourcePath, FileName: "brief.txt"})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	for name, body := range map[string]map[string]any{
		"missing_ref": {"direction": "input", "path": "docs/spec.md"},
		"both_refs":   {"direction": "input", "path": "docs/spec.md", "space_id": spaceID, "resource_id": resourceID},
	} {
		resp, err := post(ts, fmt.Sprintf("/actions/%d/io-decls", actionID), body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", name, resp.StatusCode)
		}
	}

	resp, err := post(ts, fmt.Sprintf("/actions/%d/io-decls", actionID), map[string]any{
		"direction": "input",
		"path":      "docs/spec.md",
		"space_id":  999999,
	})
	if err != nil {
		t.Fatalf("missing space: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing space, got %d", resp.StatusCode)
	}

	resp, err = post(ts, fmt.Sprintf("/actions/%d/io-decls", actionID), map[string]any{
		"direction":   "input",
		"path":        "docs/spec.md",
		"space_id":    spaceID,
		"media_type":  "text/markdown",
		"description": "spec",
		"required":    true,
	})
	if err != nil {
		t.Fatalf("create space decl: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating space decl, got %d", resp.StatusCode)
	}
	var spaceDecl core.ActionIODecl
	if err := decodeJSON(resp, &spaceDecl); err != nil {
		t.Fatalf("decode space decl: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/actions/%d/io-decls", actionID), map[string]any{
		"direction":   "input",
		"path":        "brief.txt",
		"resource_id": resourceID,
		"media_type":  "text/plain",
	})
	if err != nil {
		t.Fatalf("create resource decl: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating resource decl, got %d", resp.StatusCode)
	}
	var resourceDecl core.ActionIODecl
	if err := decodeJSON(resp, &resourceDecl); err != nil {
		t.Fatalf("decode resource decl: %v", err)
	}

	resp, err = get(ts, fmt.Sprintf("/actions/%d/io-decls", actionID))
	if err != nil {
		t.Fatalf("list decls: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing decls, got %d", resp.StatusCode)
	}
	var decls []*core.ActionIODecl
	if err := decodeJSON(resp, &decls); err != nil {
		t.Fatalf("decode decls: %v", err)
	}
	if len(decls) != 2 {
		t.Fatalf("expected 2 decls, got %d", len(decls))
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/io-decls/%d", spaceDecl.ID), nil)
	if err != nil {
		t.Fatalf("build decl delete request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete decl: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 deleting decl, got %d", resp.StatusCode)
	}
}

func TestAPI_WorkItemResourceUploadDownloadAndDelete(t *testing.T) {
	dataDir := t.TempDir()
	h, ts := setupAPIWithDataDir(t, dataDir)
	ctx := context.Background()

	projectID, err := h.store.CreateProject(ctx, &core.Project{Name: "file-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{ProjectID: &projectID, Title: "upload-item", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	resp, err := postMultipart(ts, fmt.Sprintf("/work-items/%d/resources", workItemID), "file", "notes.md", "text/markdown", []byte("# notes\nhello"))
	if err != nil {
		t.Fatalf("upload resource: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 uploading resource, got %d", resp.StatusCode)
	}
	var resource core.Resource
	if err := decodeJSON(resp, &resource); err != nil {
		t.Fatalf("decode resource: %v", err)
	}

	if _, err := os.Stat(resource.URI); err != nil {
		t.Fatalf("expected uploaded file to exist: %v", err)
	}

	resp, err = get(ts, fmt.Sprintf("/work-items/%d/resources", workItemID))
	if err != nil {
		t.Fatalf("list work item resources: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing work item resources, got %d", resp.StatusCode)
	}
	var resources []*core.Resource
	if err := decodeJSON(resp, &resources); err != nil {
		t.Fatalf("decode work item resources: %v", err)
	}
	if len(resources) != 1 || resources[0].ID != resource.ID {
		t.Fatalf("unexpected work item resources: %+v", resources)
	}

	resp, err = get(ts, fmt.Sprintf("/resources/%d", resource.ID))
	if err != nil {
		t.Fatalf("get resource: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 getting resource, got %d", resp.StatusCode)
	}
	var detail core.Resource
	if err := decodeJSON(resp, &detail); err != nil {
		t.Fatalf("decode resource detail: %v", err)
	}
	if detail.FileName != "notes.md" || detail.MimeType != "text/markdown" {
		t.Fatalf("unexpected resource detail: %+v", detail)
	}

	resp, err = get(ts, fmt.Sprintf("/resources/%d/download", resource.ID))
	if err != nil {
		t.Fatalf("download resource: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 downloading resource, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "hello") {
		t.Fatalf("unexpected download content: %q", string(body))
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "notes.md") {
		t.Fatalf("unexpected content disposition: %q", resp.Header.Get("Content-Disposition"))
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/resources/%d", resource.ID), nil)
	if err != nil {
		t.Fatalf("build resource delete request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete resource: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 deleting resource, got %d", resp.StatusCode)
	}
	if _, err := os.Stat(resource.URI); !os.IsNotExist(err) {
		t.Fatalf("expected uploaded file removed after delete, got err=%v", err)
	}
}

func TestAPI_WorkItemResourceUploadRejectsBoundaryCases(t *testing.T) {
	dataDir := t.TempDir()
	_, ts := setupAPIWithDataDir(t, dataDir)

	resp, err := postMultipart(ts, "/work-items/999999/resources", "file", "notes.md", "text/markdown", []byte("missing owner"))
	if err != nil {
		t.Fatalf("upload missing owner resource: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing work item, got %d", resp.StatusCode)
	}

	resp, err = postMultipart(ts, "/work-items/999999/resources", "file", "malware.exe", "application/x-msdownload", []byte("boom"))
	if err != nil {
		t.Fatalf("upload invalid mime resource: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected owner lookup to happen before mime validation, got %d", resp.StatusCode)
	}

	h, ts2 := setupAPIWithDataDir(t, t.TempDir())
	ctx := context.Background()
	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{Title: "owned-item", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	resp, err = postMultipart(ts2, fmt.Sprintf("/work-items/%d/resources", workItemID), "file", "malware.exe", "application/x-msdownload", []byte("boom"))
	if err != nil {
		t.Fatalf("upload invalid mime for existing owner: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mime, got %d", resp.StatusCode)
	}

	resp, err = postMultipart(ts2, fmt.Sprintf("/work-items/%d/resources", workItemID), "file", "notes.txt", "text/plain", make([]byte, maxResourceSize+1))
	if err != nil {
		t.Fatalf("upload oversized resource: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized resource, got %d", resp.StatusCode)
	}
}

func TestAPI_MessageAndRunResourceRoutes(t *testing.T) {
	dataDir := t.TempDir()
	h, ts := setupAPIWithDataDir(t, dataDir)
	ctx := context.Background()

	threadID, err := h.store.CreateThread(ctx, &core.Thread{Title: "resource-thread", Status: core.ThreadActive})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	messageID, err := h.store.CreateThreadMessage(ctx, &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: "user-1",
		Role:     "human",
		Content:  "message body",
	})
	if err != nil {
		t.Fatalf("create thread message: %v", err)
	}

	resp, err := postMultipart(ts, fmt.Sprintf("/messages/%d/resources", messageID), "file", "reply.txt", "text/plain", []byte("message attachment"))
	if err != nil {
		t.Fatalf("upload message resource: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 uploading message resource, got %d", resp.StatusCode)
	}
	var messageResource core.Resource
	if err := decodeJSON(resp, &messageResource); err != nil {
		t.Fatalf("decode message resource: %v", err)
	}
	if messageResource.ProjectID != 0 {
		t.Fatalf("expected project_id=0 for unfocused thread message resource, got %d", messageResource.ProjectID)
	}

	resp, err = get(ts, fmt.Sprintf("/messages/%d/resources", messageID))
	if err != nil {
		t.Fatalf("list message resources: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing message resources, got %d", resp.StatusCode)
	}
	var messageResources []*core.Resource
	if err := decodeJSON(resp, &messageResources); err != nil {
		t.Fatalf("decode message resources: %v", err)
	}
	if len(messageResources) != 1 || messageResources[0].ID != messageResource.ID {
		t.Fatalf("unexpected message resources: %+v", messageResources)
	}

	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{Title: "run item", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := h.store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "impl", Type: core.ActionExec, Status: core.ActionPending})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := h.store.CreateRun(ctx, &core.Run{ActionID: actionID, WorkItemID: workItemID, Status: core.RunSucceeded, Attempt: 1})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := h.store.CreateResource(ctx, &core.Resource{
		ProjectID:   0,
		RunID:       &runID,
		StorageKind: "local",
		URI:         filepath.Join(dataDir, "manual.txt"),
		Role:        "output",
		FileName:    "manual.txt",
		MimeType:    "text/plain",
	}); err != nil {
		t.Fatalf("create run resource: %v", err)
	}

	resp, err = get(ts, fmt.Sprintf("/runs/%d/resources", runID))
	if err != nil {
		t.Fatalf("list run resources: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing run resources, got %d", resp.StatusCode)
	}
	var runResources []*core.Resource
	if err := decodeJSON(resp, &runResources); err != nil {
		t.Fatalf("decode run resources: %v", err)
	}
	if len(runResources) != 1 || runResources[0].FileName != "manual.txt" {
		t.Fatalf("unexpected run resources: %+v", runResources)
	}
}

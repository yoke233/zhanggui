package outbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type faultCache struct {
	data           map[string]string
	getErrByKey    map[string]error
	setErrByKey    map[string]error
	setErrPrefixes map[string]error
}

func newFaultCache() *faultCache {
	return &faultCache{
		data:           make(map[string]string),
		getErrByKey:    make(map[string]error),
		setErrByKey:    make(map[string]error),
		setErrPrefixes: make(map[string]error),
	}
}

func (c *faultCache) Get(_ context.Context, key string) (string, bool, error) {
	if err, ok := c.getErrByKey[key]; ok {
		return "", false, err
	}
	value, found := c.data[key]
	return value, found, nil
}

func (c *faultCache) Set(_ context.Context, key string, value string, _ time.Duration) error {
	if err, ok := c.setErrByKey[key]; ok {
		return err
	}
	for prefix, err := range c.setErrPrefixes {
		if strings.HasPrefix(key, prefix) {
			return err
		}
	}
	c.data[key] = value
	return nil
}

func (c *faultCache) Delete(_ context.Context, key string) error {
	delete(c.data, key)
	return nil
}

func writeLeadWorkflowFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "workflow.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func TestLeadSyncOnceRoleNotEnabled(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowFile(t, `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["qa"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 1
mode = "owner"
writeback = "full"
listen_labels = ["to:backend"]
`)

	_, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "role backend is not enabled") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
}

func TestLeadSyncOnceGroupMissing(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowFile(t, `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.qa]
role = "qa"
max_concurrent = 1
mode = "owner"
writeback = "full"
listen_labels = ["to:qa"]
`)

	_, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "group config is required for role backend") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
}

func TestLeadSyncOnceNonSQLiteBackend(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowFile(t, `
version = 2

[outbox]
backend = "github"
path = "none"

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 1
mode = "owner"
writeback = "full"
listen_labels = ["to:backend"]
`)

	_, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "lead only supports sqlite backend") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
}

func TestLeadSyncOnceCacheGetCursorError(t *testing.T) {
	baseSvc, _ := setupService(t)
	cache := newFaultCache()
	cache.getErrByKey[leadCursorKey("backend")] = errors.New("cache get failed")
	svc := &Service{
		repo:  baseSvc.repo,
		uow:   baseSvc.uow,
		cache: cache,
	}

	_, err := svc.LeadSyncOnce(context.Background(), LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: writeTestWorkflow(t),
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "cache get failed") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
}

func TestLeadSyncOnceCacheSetCursorError(t *testing.T) {
	baseSvc, _ := setupService(t)
	cache := newFaultCache()
	cache.setErrByKey[leadCursorKey("backend")] = errors.New("cache set cursor failed")
	svc := &Service{
		repo:  baseSvc.repo,
		uow:   baseSvc.uow,
		cache: cache,
	}
	ctx := context.Background()

	issueRef := createLeadClaimedIssue(t, svc, ctx, "cursor set fail issue", "body", []string{"to:backend", "state:todo", "needs-human"})
	_ = issueRef

	_, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: writeTestWorkflow(t),
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "cache set cursor failed") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
}

func TestLeadSyncOnceCacheSetActiveRunError(t *testing.T) {
	baseSvc, _ := setupService(t)
	cache := newFaultCache()
	cache.setErrPrefixes["lead:backend:active_run:"] = errors.New("cache set active run failed")
	svc := &Service{
		repo:  baseSvc.repo,
		uow:   baseSvc.uow,
		cache: cache,
	}
	ctx := context.Background()

	issueRef := createLeadClaimedIssue(t, svc, ctx, "active run set fail issue", "body", []string{"to:backend", "state:todo"})
	_ = issueRef
	workerCalled := false
	svc.workerInvoker = func(_ context.Context, _ invokeWorkerInput) error {
		workerCalled = true
		return nil
	}

	_, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: writeTestWorkflow(t),
		EventBatch:   100,
	})
	if err == nil || !strings.Contains(err.Error(), "cache set active run failed") {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if workerCalled {
		t.Fatalf("worker should not be called when setting active run fails")
	}
}

package sqlite

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestFeatureEntryCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	proj := &core.Project{Name: "entry-test"}
	pid, _ := store.CreateProject(ctx, proj)

	// Create entries.
	e1 := &core.FeatureEntry{
		ProjectID:   pid,
		Key:         "auth.login.success",
		Description: "User can log in with valid credentials",
		Tags:        []string{"auth", "core"},
	}
	id1, err := store.CreateFeatureEntry(ctx, e1)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if id1 == 0 {
		t.Fatal("expected non-zero entry ID")
	}
	if e1.Status != core.FeaturePending {
		t.Fatalf("default status = %q, want %q", e1.Status, core.FeaturePending)
	}

	e2 := &core.FeatureEntry{
		ProjectID:   pid,
		Key:         "auth.login.fail",
		Description: "Invalid credentials show error",
		Status:      core.FeaturePass,
		Tags:        []string{"auth"},
	}
	_, err = store.CreateFeatureEntry(ctx, e2)
	if err != nil {
		t.Fatalf("create entry2: %v", err)
	}

	// Duplicate key should fail.
	dupEntry := &core.FeatureEntry{ProjectID: pid, Key: "auth.login.success"}
	_, err = store.CreateFeatureEntry(ctx, dupEntry)
	if err != core.ErrDuplicateEntryKey {
		t.Fatalf("expected ErrDuplicateEntryKey, got %v", err)
	}

	// Get by ID.
	got, err := store.GetFeatureEntry(ctx, id1)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if got.Key != "auth.login.success" {
		t.Fatalf("key = %q, want %q", got.Key, "auth.login.success")
	}

	// Get by key.
	got2, err := store.GetFeatureEntryByKey(ctx, pid, "auth.login.fail")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if got2.Status != core.FeaturePass {
		t.Fatalf("status = %q, want %q", got2.Status, core.FeaturePass)
	}

	// List all.
	entries, err := store.ListFeatureEntries(ctx, core.FeatureEntryFilter{ProjectID: pid})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("list len = %d, want 2", len(entries))
	}

	// List by status filter.
	passStatus := core.FeaturePass
	filtered, err := store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: pid,
		Status:    &passStatus,
	})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}

	// Update entry.
	got.Description = "Updated description"
	got.Status = core.FeatureFail
	if err := store.UpdateFeatureEntry(ctx, got); err != nil {
		t.Fatalf("update entry: %v", err)
	}
	got3, _ := store.GetFeatureEntry(ctx, id1)
	if got3.Description != "Updated description" || got3.Status != core.FeatureFail {
		t.Fatalf("update not persisted")
	}

	// Update status only.
	if err := store.UpdateFeatureEntryStatus(ctx, id1, core.FeaturePass); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got4, _ := store.GetFeatureEntry(ctx, id1)
	if got4.Status != core.FeaturePass {
		t.Fatalf("status = %q, want %q", got4.Status, core.FeaturePass)
	}

	// Delete entry.
	if err := store.DeleteFeatureEntry(ctx, id1); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	_, err = store.GetFeatureEntry(ctx, id1)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestCountFeatureEntriesByStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	proj := &core.Project{Name: "count-test"}
	pid, _ := store.CreateProject(ctx, proj)

	entries := []struct {
		key    string
		status core.FeatureStatus
	}{
		{"f1", core.FeaturePending},
		{"f2", core.FeaturePending},
		{"f3", core.FeaturePass},
		{"f4", core.FeatureFail},
		{"f5", core.FeaturePass},
		{"f6", core.FeatureSkipped},
	}
	for _, e := range entries {
		_, err := store.CreateFeatureEntry(ctx, &core.FeatureEntry{
			ProjectID: pid,
			Key:       e.key,
			Status:    e.status,
		})
		if err != nil {
			t.Fatalf("create %s: %v", e.key, err)
		}
	}

	counts, err := store.CountFeatureEntriesByStatus(ctx, pid)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if counts[core.FeaturePending] != 2 {
		t.Errorf("pending = %d, want 2", counts[core.FeaturePending])
	}
	if counts[core.FeaturePass] != 2 {
		t.Errorf("pass = %d, want 2", counts[core.FeaturePass])
	}
	if counts[core.FeatureFail] != 1 {
		t.Errorf("fail = %d, want 1", counts[core.FeatureFail])
	}
	if counts[core.FeatureSkipped] != 1 {
		t.Errorf("skipped = %d, want 1", counts[core.FeatureSkipped])
	}
}

func TestFeatureEntryTagsFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	proj := &core.Project{Name: "tags-test"}
	pid, _ := store.CreateProject(ctx, proj)

	// Create entries with different tags.
	store.CreateFeatureEntry(ctx, &core.FeatureEntry{
		ProjectID: pid, Key: "auth.login", Tags: []string{"auth", "core"},
	})
	store.CreateFeatureEntry(ctx, &core.FeatureEntry{
		ProjectID: pid, Key: "auth.logout", Tags: []string{"auth"},
	})
	store.CreateFeatureEntry(ctx, &core.FeatureEntry{
		ProjectID: pid, Key: "payment.checkout", Tags: []string{"payment", "core"},
	})
	store.CreateFeatureEntry(ctx, &core.FeatureEntry{
		ProjectID: pid, Key: "settings.profile", Tags: []string{"settings"},
	})

	// Filter by single tag.
	entries, err := store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: pid, Tags: []string{"auth"},
	})
	if err != nil {
		t.Fatalf("filter by auth: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("auth tag: got %d entries, want 2", len(entries))
	}

	// Filter by multiple tags (AND).
	entries, err = store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: pid, Tags: []string{"auth", "core"},
	})
	if err != nil {
		t.Fatalf("filter by auth+core: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("auth+core tags: got %d entries, want 1", len(entries))
	}
	if len(entries) > 0 && entries[0].Key != "auth.login" {
		t.Errorf("expected auth.login, got %s", entries[0].Key)
	}

	// Filter by tag with no matches.
	entries, err = store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: pid, Tags: []string{"nonexistent"},
	})
	if err != nil {
		t.Fatalf("filter by nonexistent: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("nonexistent tag: got %d entries, want 0", len(entries))
	}
}

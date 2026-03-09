package teamleader

import (
	"reflect"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestAreDependenciesMet(t *testing.T) {
	issues := map[string]*core.Issue{
		"A": {ID: "A", Status: core.IssueStatusDone},
		"B": {ID: "B", Status: core.IssueStatusDone},
		"C": {ID: "C", Status: core.IssueStatusExecuting},
	}

	lookup := func(id string) *core.Issue {
		return issues[id]
	}

	if !areDependenciesMet([]string{"A", "B"}, core.FailBlock, lookup) {
		t.Fatal("expected deps A,B to be met")
	}
	if areDependenciesMet([]string{"A", "C"}, core.FailBlock, lookup) {
		t.Fatal("expected deps A,C to not be met")
	}
	if !areDependenciesMet(nil, core.FailBlock, lookup) {
		t.Fatal("expected nil deps to be met")
	}
	if areDependenciesMet([]string{"Z"}, core.FailBlock, lookup) {
		t.Fatal("expected unknown dep to not be met")
	}
}

func TestAreDependenciesMet_FailedDependencyRespectsFailPolicy(t *testing.T) {
	issues := map[string]*core.Issue{
		"A": {ID: "A", Status: core.IssueStatusDone},
		"B": {ID: "B", Status: core.IssueStatusFailed},
		"C": {ID: "C", Status: core.IssueStatusAbandoned},
	}

	lookup := func(id string) *core.Issue {
		return issues[id]
	}

	if !areDependenciesMet([]string{"B"}, core.FailSkip, lookup) {
		t.Fatal("expected failed dep to be treated as satisfied under FailSkip")
	}
	if areDependenciesMet([]string{"B"}, core.FailBlock, lookup) {
		t.Fatal("expected failed dep to block under FailBlock")
	}
	if !areDependenciesMet([]string{"C"}, core.FailSkip, lookup) {
		t.Fatal("expected abandoned dep to be treated as satisfied under FailSkip")
	}
	if areDependenciesMet([]string{"B"}, core.FailHuman, lookup) {
		t.Fatal("expected failed dep to block under FailHuman")
	}
	if !areDependenciesMet([]string{"A", "B"}, core.FailSkip, lookup) {
		t.Fatal("expected mixed done+failed deps to be satisfied under FailSkip")
	}
}

func TestEffectiveDependsOn_Sequential(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-effective-sequential")
	parent := core.Issue{
		ID:           "parent-seq",
		Title:        "parent",
		ProjectID:    project.ID,
		Template:     "epic",
		State:        core.IssueStateOpen,
		Status:       core.IssueStatusDecomposed,
		ChildrenMode: core.ChildrenModeSequential,
	}
	children := []core.Issue{
		{ID: "child-a", ProjectID: project.ID, ParentID: parent.ID, Title: "A", Template: "standard", State: core.IssueStateOpen, Status: core.IssueStatusQueued, Priority: 30},
		{ID: "child-b", ProjectID: project.ID, ParentID: parent.ID, Title: "B", Template: "standard", State: core.IssueStateOpen, Status: core.IssueStatusQueued, Priority: 20},
		{ID: "child-c", ProjectID: project.ID, ParentID: parent.ID, Title: "C", Template: "standard", State: core.IssueStateOpen, Status: core.IssueStatusQueued, Priority: 10},
	}
	for _, issue := range append([]core.Issue{parent}, children...) {
		issue := issue
		if err := store.CreateIssue(&issue); err != nil {
			t.Fatalf("CreateIssue(%s) error = %v", issue.ID, err)
		}
	}

	loadedParent, _ := store.GetIssue(parent.ID)
	loadedA, _ := store.GetIssue("child-a")
	loadedB, _ := store.GetIssue("child-b")
	loadedC, _ := store.GetIssue("child-c")
	rs := newRunningSession("session-effective-sequential", project.ID, []*core.Issue{loadedParent, loadedA, loadedB, loadedC})
	scheduler := NewDepScheduler(store, nil, nil, nil, 1)

	if got := scheduler.effectiveDependsOn(rs, loadedB); !reflect.DeepEqual(got, []string{"child-a"}) {
		t.Fatalf("effective deps for B = %#v, want [child-a]", got)
	}
	if got := scheduler.effectiveDependsOn(rs, loadedC); !reflect.DeepEqual(got, []string{"child-b"}) {
		t.Fatalf("effective deps for C = %#v, want [child-b]", got)
	}
}

func TestEffectiveDependsOn_Parallel(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-effective-parallel")
	parent := core.Issue{
		ID:           "parent-parallel",
		Title:        "parent",
		ProjectID:    project.ID,
		Template:     "epic",
		State:        core.IssueStateOpen,
		Status:       core.IssueStatusDecomposed,
		ChildrenMode: core.ChildrenModeParallel,
	}
	child := core.Issue{
		ID:        "child-parallel",
		ProjectID: project.ID,
		ParentID:  parent.ID,
		Title:     "child",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusQueued,
		DependsOn: []string{"explicit-dep"},
	}
	for _, issue := range []core.Issue{parent, child} {
		issue := issue
		if err := store.CreateIssue(&issue); err != nil {
			t.Fatalf("CreateIssue(%s) error = %v", issue.ID, err)
		}
	}

	loadedParent, _ := store.GetIssue(parent.ID)
	loadedChild, _ := store.GetIssue(child.ID)
	rs := newRunningSession("session-effective-parallel", project.ID, []*core.Issue{loadedParent, loadedChild})
	scheduler := NewDepScheduler(store, nil, nil, nil, 1)

	if got := scheduler.effectiveDependsOn(rs, loadedChild); !reflect.DeepEqual(got, []string{"explicit-dep"}) {
		t.Fatalf("effective deps for parallel child = %#v, want original deps", got)
	}
}

func TestEffectiveDependsOn_NoParent(t *testing.T) {
	scheduler := NewDepScheduler(newSchedulerTestStore(t), nil, nil, nil, 1)
	defer scheduler.store.Close()

	issue := &core.Issue{
		ID:        "standalone",
		DependsOn: []string{"dep-a", "dep-b"},
	}
	rs := newRunningSession("session-no-parent", "proj-no-parent", []*core.Issue{issue})

	if got := scheduler.effectiveDependsOn(rs, issue); !reflect.DeepEqual(got, issue.DependsOn) {
		t.Fatalf("effective deps for standalone issue = %#v, want %#v", got, issue.DependsOn)
	}
}

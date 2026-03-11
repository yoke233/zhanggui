package scm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
)

func TestCodeupProviderDetect_SSHRemote(t *testing.T) {
	provider := NewCodeupProvider(CodeupProviderConfig{})

	repo, ok, err := provider.Detect(context.Background(), "git@codeup.aliyun.com:5f6ea0829cffa29cfdd39a7f/xiaoin/xiaoin-rag-service.git")
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if !ok {
		t.Fatal("expected codeup remote to be detected")
	}
	if repo.Host != "codeup.aliyun.com" {
		t.Fatalf("host = %q", repo.Host)
	}
	if repo.Namespace != "5f6ea0829cffa29cfdd39a7f/xiaoin" {
		t.Fatalf("namespace = %q", repo.Namespace)
	}
	if repo.Name != "xiaoin-rag-service" {
		t.Fatalf("name = %q", repo.Name)
	}
}

func TestCodeupProviderEnsureOpen_CreatesChangeRequest(t *testing.T) {
	var gotPath string
	var gotToken string
	var gotBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/oapi/v1/codeup/organizations/5f6ea0829cffa29cfdd39a7f/repositories/5f6ea0829cffa29cfdd39a7f%2Fxiaoin%2Fxiaoin-rag-service/changeRequests", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("x-yunxiao-token")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"localId":17,"webUrl":"https://codeup.aliyun.com/cr/17","sha":"abc123"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider := NewCodeupProvider(CodeupProviderConfig{
		Token:  "codeup-token",
		Domain: srv.URL,
	})
	provider.httpClient = srv.Client()

	cr, created, err := provider.EnsureOpen(context.Background(), flowapp.ChangeRequestRepo{
		Kind:      "codeup",
		Host:      "codeup.aliyun.com",
		Namespace: "5f6ea0829cffa29cfdd39a7f/xiaoin",
		Name:      "xiaoin-rag-service",
	}, flowapp.EnsureOpenInput{
		Head:  "feature/test",
		Base:  "main",
		Title: "test title",
		Body:  "desc",
		Extra: map[string]any{
			"project_id":            int64(2369234),
			"reviewer_user_ids":     []string{"u1", "u2"},
			"trigger_ai_review_run": true,
			"work_item_ids":         "722200214032b6b31e6f1434ab",
		},
	})
	if err != nil {
		t.Fatalf("EnsureOpen error: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if cr.Number != 17 {
		t.Fatalf("number = %d", cr.Number)
	}
	if cr.URL != "https://codeup.aliyun.com/cr/17" {
		t.Fatalf("url = %q", cr.URL)
	}
	if gotToken != "codeup-token" {
		t.Fatalf("token header = %q", gotToken)
	}
	if !strings.Contains(gotPath, "/organizations/5f6ea0829cffa29cfdd39a7f/") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["sourceProjectId"] != float64(2369234) {
		t.Fatalf("sourceProjectId = %#v", gotBody["sourceProjectId"])
	}
	if gotBody["targetProjectId"] != float64(2369234) {
		t.Fatalf("targetProjectId = %#v", gotBody["targetProjectId"])
	}
	if gotBody["repositoryId"] != nil {
		t.Fatalf("unexpected repositoryId in body: %#v", gotBody["repositoryId"])
	}
}

func TestCodeupProviderMerge_UsesConfiguredEndpoint(t *testing.T) {
	var gotBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/oapi/v1/codeup/organizations/5f6ea0829cffa29cfdd39a7f/repositories/5f6ea0829cffa29cfdd39a7f%2Fxiaoin%2Fxiaoin-rag-service/changeRequests/23/merge", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider := NewCodeupProvider(CodeupProviderConfig{
		Token:  "codeup-token",
		Domain: srv.URL,
	})
	provider.httpClient = srv.Client()

	err := provider.Merge(context.Background(), flowapp.ChangeRequestRepo{
		Kind:      "codeup",
		Host:      "codeup.aliyun.com",
		Namespace: "5f6ea0829cffa29cfdd39a7f/xiaoin",
		Name:      "xiaoin-rag-service",
	}, 23, flowapp.MergeInput{
		Method:        "merge",
		CommitMessage: "merge now",
		Extra: map[string]any{
			"project_id":           int64(2369234),
			"remove_source_branch": true,
		},
	})
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	if gotBody["mergeType"] != "no-fast-forward" {
		t.Fatalf("mergeType = %#v", gotBody["mergeType"])
	}
	if gotBody["removeSourceBranch"] != true {
		t.Fatalf("removeSourceBranch = %#v", gotBody["removeSourceBranch"])
	}
}

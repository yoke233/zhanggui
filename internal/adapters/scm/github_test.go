package scm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
)

func TestGitHubProviderMerge_405NotMergedReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/demo/pulls/7/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method for merge: %s", r.Method)
		}
		http.Error(w, `{"message":"Merge conflict"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/repos/acme/demo/pulls/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method for get: %s", r.Method)
		}
		_, _ = fmt.Fprint(w, `{"number":7,"merged":false}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider := &GitHubProvider{
		token:      "test-token",
		httpClient: srv.Client(),
		baseURL:    srv.URL + "/",
	}

	err := provider.Merge(context.Background(), flowapp.ChangeRequestRepo{
		Namespace: "acme",
		Name:      "demo",
	}, 7, flowapp.MergeInput{Method: "squash"})
	if err == nil {
		t.Fatal("expected merge error when PR is not merged")
	}
}

func TestGitHubProviderMerge_405AlreadyMergedReturnsNil(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/demo/pulls/8/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method for merge: %s", r.Method)
		}
		http.Error(w, `{"message":"Pull Request is not mergeable"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/repos/acme/demo/pulls/8", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method for get: %s", r.Method)
		}
		_, _ = fmt.Fprint(w, `{"number":8,"merged":true}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider := &GitHubProvider{
		token:      "test-token",
		httpClient: srv.Client(),
		baseURL:    srv.URL + "/",
	}

	if err := provider.Merge(context.Background(), flowapp.ChangeRequestRepo{
		Namespace: "acme",
		Name:      "demo",
	}, 8, flowapp.MergeInput{Method: "squash"}); err != nil {
		t.Fatalf("expected merged PR to be treated as success, got: %v", err)
	}
}

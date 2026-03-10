package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ghapi "github.com/google/go-github/v68/github"
)

func TestGitHubService_CreateIssue_Success(t *testing.T) {
	service := newTestGitHubService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/acme/demo/issues" {
			t.Fatalf("expected path /repos/acme/demo/issues, got %s", r.URL.Path)
		}

		var payload struct {
			Title  string   `json:"title"`
			Body   string   `json:"body"`
			Labels []string `json:"labels"`
		}
		decodeJSONBody(t, r, &payload)

		expected := []string{"status: ready", "plan: p3"}
		if payload.Title != "实现 gh-4" {
			t.Fatalf("expected title %q, got %q", "实现 gh-4", payload.Title)
		}
		if payload.Body != "实现 GitHubService" {
			t.Fatalf("expected body %q, got %q", "实现 GitHubService", payload.Body)
		}
		if !reflect.DeepEqual(payload.Labels, expected) {
			t.Fatalf("expected labels %v, got %v", expected, payload.Labels)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":101,"html_url":"https://github.com/acme/demo/issues/101"}`))
	})

	issue, err := service.CreateIssue(context.Background(), CreateIssueInput{
		Title:  "实现 gh-4",
		Body:   "实现 GitHubService",
		Labels: []string{"status: ready", "plan: p3"},
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}
	if issue.GetNumber() != 101 {
		t.Fatalf("expected issue number 101, got %d", issue.GetNumber())
	}
}

func TestGitHubService_UpdateLabels_ReplaceStatusLabel(t *testing.T) {
	var (
		callCount    int
		removedLabel string
		addedLabels  []string
	)

	service := newTestGitHubService(t, func(w http.ResponseWriter, r *http.Request) {
		switch callCount {
		case 0:
			if r.Method != http.MethodGet {
				t.Fatalf("expected method GET, got %s", r.Method)
			}
			if r.URL.Path != "/repos/acme/demo/issues/42/labels" {
				t.Fatalf("expected path /repos/acme/demo/issues/42/labels, got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"status: blocked"},{"name":"priority: high"}]`))
		case 1:
			if r.Method != http.MethodDelete {
				t.Fatalf("expected method DELETE, got %s", r.Method)
			}
			if !strings.HasPrefix(r.URL.Path, "/repos/acme/demo/issues/42/labels/") {
				t.Fatalf("expected delete label path prefix, got %s", r.URL.Path)
			}
			lastSlash := strings.LastIndex(r.URL.Path, "/")
			labelPart := r.URL.Path[lastSlash+1:]
			decoded, err := url.PathUnescape(labelPart)
			if err != nil {
				t.Fatalf("PathUnescape failed: %v", err)
			}
			removedLabel = decoded
			w.WriteHeader(http.StatusOK)
		case 2:
			if r.Method != http.MethodPost {
				t.Fatalf("expected method POST, got %s", r.Method)
			}
			if r.URL.Path != "/repos/acme/demo/issues/42/labels" {
				t.Fatalf("expected path /repos/acme/demo/issues/42/labels, got %s", r.URL.Path)
			}
			decodeJSONBody(t, r, &addedLabels)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"status: in-progress"},{"name":"plan: p3"}]`))
		default:
			t.Fatalf("unexpected extra request: %s %s", r.Method, r.URL.Path)
		}
		callCount++
	})

	err := service.UpdateIssueLabels(context.Background(), 42, []string{"status: in-progress", "plan: p3"})
	if err != nil {
		t.Fatalf("UpdateIssueLabels returned error: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 requests, got %d", callCount)
	}
	if removedLabel != "status: blocked" {
		t.Fatalf("expected removed status label %q, got %q", "status: blocked", removedLabel)
	}

	expectedLabels := []string{"status: in-progress", "plan: p3"}
	if !reflect.DeepEqual(addedLabels, expectedLabels) {
		t.Fatalf("expected added labels %v, got %v", expectedLabels, addedLabels)
	}
}

func TestGitHubService_AddIssueComment_Success(t *testing.T) {
	service := newTestGitHubService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/acme/demo/issues/7/comments" {
			t.Fatalf("expected path /repos/acme/demo/issues/7/comments, got %s", r.URL.Path)
		}

		var payload struct {
			Body string `json:"body"`
		}
		decodeJSONBody(t, r, &payload)
		if payload.Body != "开始执行 implement 阶段" {
			t.Fatalf("expected comment body %q, got %q", "开始执行 implement 阶段", payload.Body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9001,"body":"开始执行 implement 阶段"}`))
	})

	comment, err := service.AddIssueComment(context.Background(), 7, "开始执行 implement 阶段")
	if err != nil {
		t.Fatalf("AddIssueComment returned error: %v", err)
	}
	if comment.GetID() != 9001 {
		t.Fatalf("expected comment id 9001, got %d", comment.GetID())
	}
}

func TestGitHubService_AddIssueComment_RetriesOnRateLimit(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS:        100,
		RateLimitBurst:      100,
		MaxRateLimitRetries: 3,
		Now:                 clock.Now,
		Sleep:               clock.Sleep,
	})

	attempts := int32(0)
	service := newTestGitHubServiceWithQueue(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/acme/demo/issues/7/comments" {
			t.Fatalf("expected path /repos/acme/demo/issues/7/comments, got %s", r.URL.Path)
		}

		current := atomic.AddInt32(&attempts, 1)
		if current == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9002,"body":"after retry"}`))
	}, queue)

	comment, err := service.AddIssueComment(context.Background(), 7, "after retry")
	if err != nil {
		t.Fatalf("AddIssueComment returned error: %v", err)
	}
	if comment.GetID() != 9002 {
		t.Fatalf("expected comment id 9002, got %d", comment.GetID())
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if got := clock.TotalSlept(); got != 2*time.Second {
		t.Fatalf("expected retry sleep 2s, got %v", got)
	}
}

func TestGitHubService_CreateDraftPR_Success(t *testing.T) {
	service := newTestGitHubService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/acme/demo/pulls" {
			t.Fatalf("expected path /repos/acme/demo/pulls, got %s", r.URL.Path)
		}

		var payload struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Head  string `json:"head"`
			Base  string `json:"base"`
			Draft bool   `json:"draft"`
		}
		decodeJSONBody(t, r, &payload)

		if payload.Title != "feat: gh-4" {
			t.Fatalf("expected title %q, got %q", "feat: gh-4", payload.Title)
		}
		if payload.Head != "feature/gh-4" || payload.Base != "main" {
			t.Fatalf("expected head/base feature/gh-4/main, got %s/%s", payload.Head, payload.Base)
		}
		if !payload.Draft {
			t.Fatal("expected draft=true")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":55,"draft":true,"html_url":"https://github.com/acme/demo/pull/55"}`))
	})

	pr, err := service.CreatePR(context.Background(), CreatePRInput{
		Title: "feat: gh-4",
		Body:  "关联 issue #101",
		Head:  "feature/gh-4",
		Base:  "main",
		Draft: true,
	})
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if pr.GetNumber() != 55 {
		t.Fatalf("expected pr number 55, got %d", pr.GetNumber())
	}
	if !pr.GetDraft() {
		t.Fatal("expected returned PR to be draft")
	}
}

func newTestGitHubService(t *testing.T, handler http.HandlerFunc) *GitHubService {
	return newTestGitHubServiceWithQueue(t, handler, nil)
}

func newTestGitHubServiceWithQueue(
	t *testing.T,
	handler http.HandlerFunc,
	queue *OutboundQueue,
) *GitHubService {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := ghapi.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	client.BaseURL = baseURL
	client.UploadURL = baseURL

	wrappedClient := &Client{
		client:     client,
		httpClient: server.Client(),
	}

	service, err := newGitHubServiceWithQueue(wrappedClient, "acme", "demo", queue)
	if err != nil {
		t.Fatalf("NewGitHubService returned error: %v", err)
	}
	return service
}

func decodeJSONBody(t *testing.T, r *http.Request, v any) {
	t.Helper()

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("unmarshal request body %q: %v", string(raw), err)
	}
}

package scm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
)

type CodeupProviderConfig struct {
	Token          string
	Domain         string
	OrganizationID string
}

type CodeupProvider struct {
	token          string
	domain         string
	organizationID string
	httpClient     *http.Client
}

const defaultCodeupAPIBaseURL = "https://openapi-rdc.aliyuncs.com"

func NewCodeupProvider(cfg CodeupProviderConfig) *CodeupProvider {
	return &CodeupProvider{
		token:          strings.TrimSpace(cfg.Token),
		domain:         normalizeCodeupDomain(cfg.Domain),
		organizationID: strings.TrimSpace(cfg.OrganizationID),
	}
}

func (p *CodeupProvider) Kind() string { return "codeup" }

func (p *CodeupProvider) Detect(_ context.Context, originURL string) (flowapp.ChangeRequestRepo, bool, error) {
	remote, err := parseCodeupRemote(originURL)
	if err != nil {
		return flowapp.ChangeRequestRepo{}, false, nil
	}
	if !p.matchesHost(remote.Host) {
		return flowapp.ChangeRequestRepo{}, false, nil
	}
	return flowapp.ChangeRequestRepo{
		Kind:      p.Kind(),
		Host:      remote.Host,
		Namespace: remote.Namespace,
		Name:      remote.Repo,
	}, true, nil
}

func (p *CodeupProvider) EnsureOpen(ctx context.Context, repo flowapp.ChangeRequestRepo, input flowapp.EnsureOpenInput) (flowapp.ChangeRequest, bool, error) {
	if strings.TrimSpace(p.token) == "" {
		return flowapp.ChangeRequest{}, false, errors.New("codeup provider token is required")
	}

	reqInfo, err := p.resolveRequest(repo, input.Extra)
	if err != nil {
		return flowapp.ChangeRequest{}, false, err
	}
	reqInfo, err = p.ensureProjectIDs(ctx, reqInfo)
	if err != nil {
		return flowapp.ChangeRequest{}, false, err
	}
	head := strings.TrimSpace(input.Head)
	base := strings.TrimSpace(input.Base)
	title := strings.TrimSpace(input.Title)
	if head == "" || base == "" || title == "" {
		return flowapp.ChangeRequest{}, false, errors.New("codeup ensure open requires title/head/base")
	}

	existing, found, err := p.findOpenChangeRequest(ctx, reqInfo, head, base, title)
	if err != nil {
		return flowapp.ChangeRequest{}, false, err
	}
	if found {
		return existing, false, nil
	}

	payload := map[string]any{
		"title":           title,
		"description":     strings.TrimSpace(input.Body),
		"sourceBranch":    head,
		"targetBranch":    base,
		"sourceProjectId": reqInfo.SourceProjectID,
		"targetProjectId": reqInfo.TargetProjectID,
	}
	if reviewers := stringSliceFromAny(input.Extra["reviewer_user_ids"]); len(reviewers) > 0 {
		payload["reviewerUserIds"] = reviewers
	}
	if trigger, ok := boolFromAny(input.Extra["trigger_ai_review_run"]); ok {
		payload["triggerAIReviewRun"] = trigger
	}
	if workItems := strings.TrimSpace(stringFromAny(input.Extra["work_item_ids"])); workItems != "" {
		payload["workItemIds"] = workItems
	}

	var created codeupChangeRequest
	if err := p.doJSON(ctx, http.MethodPost, reqInfo.createURL(), payload, &created); err != nil {
		return flowapp.ChangeRequest{}, false, fmt.Errorf("codeup create change request failed: %w", err)
	}
	cr := created.toChangeRequest()
	if cr.Number <= 0 {
		return flowapp.ChangeRequest{}, false, errors.New("codeup create change request returned empty localId")
	}
	cr.Metadata = reqInfo.metadata()
	return cr, true, nil
}

func (p *CodeupProvider) Merge(ctx context.Context, repo flowapp.ChangeRequestRepo, number int, input flowapp.MergeInput) error {
	if strings.TrimSpace(p.token) == "" {
		return errors.New("codeup provider token is required")
	}
	if number <= 0 {
		return errors.New("codeup merge requires a positive change request number")
	}

	reqInfo, err := p.resolveRequest(repo, input.Extra)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"mergeType":          codeupMergeMethod(input.Method),
		"removeSourceBranch": false,
	}
	if removeSourceBranch, ok := boolFromAny(input.Extra["remove_source_branch"]); ok {
		payload["removeSourceBranch"] = removeSourceBranch
	}
	mergeMessage := strings.TrimSpace(input.CommitMessage)
	if mergeMessage == "" {
		mergeMessage = strings.TrimSpace(input.CommitTitle)
	}
	if mergeMessage != "" {
		payload["mergeMessage"] = mergeMessage
	}

	err = p.doJSON(ctx, http.MethodPost, reqInfo.mergeURL(number), payload, nil)
	if err == nil {
		return nil
	}

	current, getErr := p.getChangeRequest(ctx, reqInfo, number)
	if getErr == nil && current.isMerged() {
		return nil
	}

	mergeErr := &flowapp.MergeError{
		Provider: p.Kind(),
		Repo:     repo,
		Number:   number,
		Message:  err.Error(),
	}
	if current != nil {
		mergeErr.URL = current.webURL()
		mergeErr.MergeableState = strings.TrimSpace(current.Status)
		mergeErr.AlreadyMerged = current.isMerged()
	}
	return mergeErr
}

func (p *CodeupProvider) GetState(ctx context.Context, repo flowapp.ChangeRequestRepo, number int) (string, error) {
	if strings.TrimSpace(p.token) == "" {
		return "", errors.New("codeup provider token is required")
	}
	if number <= 0 {
		return "", errors.New("codeup get state requires a positive change request number")
	}
	reqInfo, err := p.resolveRequest(repo, nil)
	if err != nil {
		return "", err
	}
	cr, err := p.getChangeRequest(ctx, reqInfo, number)
	if err != nil {
		return "", fmt.Errorf("codeup get CR #%d failed: %w", number, err)
	}
	if cr.isMerged() {
		return "merged", nil
	}
	state := strings.ToLower(strings.TrimSpace(cr.Status))
	if state == "" || state == "opened" {
		return "open", nil
	}
	return state, nil
}

type codeupRequestInfo struct {
	BaseURL         string
	OrganizationID  string
	RepositoryID    string
	SourceProjectID int64
	TargetProjectID int64
}

func (i codeupRequestInfo) createURL() string {
	return strings.TrimRight(i.BaseURL, "/") + fmt.Sprintf("/oapi/v1/codeup/organizations/%s/repositories/%s/changeRequests",
		url.PathEscape(i.OrganizationID), url.PathEscape(i.RepositoryID))
}

func (i codeupRequestInfo) repositoryURL() string {
	return strings.TrimRight(i.BaseURL, "/") + fmt.Sprintf("/oapi/v1/codeup/organizations/%s/repositories/%s",
		url.PathEscape(i.OrganizationID), url.PathEscape(i.RepositoryID))
}

func (i codeupRequestInfo) mergeURL(number int) string {
	return strings.TrimRight(i.BaseURL, "/") + fmt.Sprintf("/oapi/v1/codeup/organizations/%s/repositories/%s/changeRequests/%d/merge",
		url.PathEscape(i.OrganizationID), url.PathEscape(i.RepositoryID), number)
}

func (i codeupRequestInfo) getURL(number int) string {
	return strings.TrimRight(i.BaseURL, "/") + fmt.Sprintf("/oapi/v1/codeup/organizations/%s/repositories/%s/changeRequests/%d",
		url.PathEscape(i.OrganizationID), url.PathEscape(i.RepositoryID), number)
}

func (i codeupRequestInfo) listURL() string {
	u := strings.TrimRight(i.BaseURL, "/") + fmt.Sprintf("/oapi/v1/codeup/organizations/%s/changeRequests",
		url.PathEscape(i.OrganizationID))
	values := url.Values{}
	if i.TargetProjectID > 0 {
		values.Set("projectIds", strconv.FormatInt(i.TargetProjectID, 10))
	} else if i.SourceProjectID > 0 {
		values.Set("projectIds", strconv.FormatInt(i.SourceProjectID, 10))
	}
	values.Set("state", "opened")
	values.Set("page", "1")
	values.Set("perPage", "100")
	return u + "?" + values.Encode()
}

func (i codeupRequestInfo) metadata() map[string]any {
	return map[string]any{
		"provider":          "codeup",
		"organization_id":   i.OrganizationID,
		"repository_id":     i.RepositoryID,
		"source_project_id": i.SourceProjectID,
		"target_project_id": i.TargetProjectID,
	}
}

type codeupChangeRequest struct {
	LocalID         int64  `json:"localId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	WebURL          string `json:"webUrl"`
	DetailURL       string `json:"detailUrl"`
	SourceBranch    string `json:"sourceBranch"`
	TargetBranch    string `json:"targetBranch"`
	SourceProjectID int64  `json:"sourceProjectId"`
	TargetProjectID int64  `json:"targetProjectId"`
	SHA             string `json:"sha"`
}

func (c codeupChangeRequest) webURL() string {
	if v := strings.TrimSpace(c.WebURL); v != "" {
		return v
	}
	return strings.TrimSpace(c.DetailURL)
}

func (c codeupChangeRequest) isMerged() bool {
	state := strings.ToLower(strings.TrimSpace(c.Status))
	return state == "merged" || state == "merge_success"
}

func (c codeupChangeRequest) toChangeRequest() flowapp.ChangeRequest {
	return flowapp.ChangeRequest{
		Number:  int(c.LocalID),
		URL:     c.webURL(),
		HeadSHA: strings.TrimSpace(c.SHA),
		Metadata: map[string]any{
			"provider": "codeup",
		},
	}
}

func (p *CodeupProvider) resolveRequest(repo flowapp.ChangeRequestRepo, extra map[string]any) (codeupRequestInfo, error) {
	if strings.TrimSpace(repo.Host) == "" {
		return codeupRequestInfo{}, errors.New("codeup repo host is required")
	}
	orgID := strings.TrimSpace(stringFromAny(extra["organization_id"]))
	if orgID == "" {
		orgID = codeupOrganizationFromNamespace(repo.Namespace)
	}
	if orgID == "" {
		orgID = p.organizationID
	}
	if orgID == "" {
		return codeupRequestInfo{}, errors.New("codeup organization_id is required")
	}

	projectID := int64FromAny(extra["project_id"])
	sourceProjectID := int64FromAny(extra["source_project_id"])
	targetProjectID := int64FromAny(extra["target_project_id"])
	if sourceProjectID == 0 {
		sourceProjectID = projectID
	}
	if targetProjectID == 0 {
		targetProjectID = projectID
	}

	repositoryID := strings.TrimSpace(stringFromAny(extra["repository_id"]))
	if repositoryID == "" {
		repositoryID = codeupRepositoryID(repo)
	}
	if repositoryID == "" {
		return codeupRequestInfo{}, errors.New("codeup repository_id is required")
	}

	baseURL := p.domain
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCodeupAPIBaseURL
	}
	return codeupRequestInfo{
		BaseURL:         baseURL,
		OrganizationID:  orgID,
		RepositoryID:    repositoryID,
		SourceProjectID: sourceProjectID,
		TargetProjectID: targetProjectID,
	}, nil
}

type codeupRepository struct {
	ID int64 `json:"id"`
}

func (p *CodeupProvider) ensureProjectIDs(ctx context.Context, reqInfo codeupRequestInfo) (codeupRequestInfo, error) {
	if reqInfo.SourceProjectID > 0 && reqInfo.TargetProjectID > 0 {
		return reqInfo, nil
	}

	var repoInfo codeupRepository
	if err := p.doJSON(ctx, http.MethodGet, reqInfo.repositoryURL(), nil, &repoInfo); err != nil {
		return codeupRequestInfo{}, fmt.Errorf("codeup resolve repository project id failed: %w", err)
	}
	if repoInfo.ID <= 0 {
		return codeupRequestInfo{}, errors.New("codeup repository lookup returned empty id")
	}
	if reqInfo.SourceProjectID == 0 {
		reqInfo.SourceProjectID = repoInfo.ID
	}
	if reqInfo.TargetProjectID == 0 {
		reqInfo.TargetProjectID = repoInfo.ID
	}
	return reqInfo, nil
}

func (p *CodeupProvider) findOpenChangeRequest(ctx context.Context, reqInfo codeupRequestInfo, head string, base string, title string) (flowapp.ChangeRequest, bool, error) {
	var raw json.RawMessage
	if err := p.doJSON(ctx, http.MethodGet, reqInfo.listURL(), nil, &raw); err != nil {
		return flowapp.ChangeRequest{}, false, err
	}
	items, err := parseCodeupChangeRequestList(raw)
	if err != nil {
		return flowapp.ChangeRequest{}, false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.SourceBranch) == head && strings.TrimSpace(item.TargetBranch) == base {
			return item.toChangeRequest(), true, nil
		}
		if title != "" && strings.TrimSpace(item.Title) == title {
			return item.toChangeRequest(), true, nil
		}
	}
	return flowapp.ChangeRequest{}, false, nil
}

func (p *CodeupProvider) getChangeRequest(ctx context.Context, reqInfo codeupRequestInfo, number int) (*codeupChangeRequest, error) {
	var out codeupChangeRequest
	if err := p.doJSON(ctx, http.MethodGet, reqInfo.getURL(number), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *CodeupProvider) doJSON(ctx context.Context, method string, rawURL string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return err
		}
		body = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(p.token) != "" {
		req.Header.Set("x-yunxiao-token", strings.TrimSpace(p.token))
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		msg := strings.TrimSpace(string(bodyBytes))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("codeup api %s %s returned %d: %s", method, rawURL, resp.StatusCode, msg)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *CodeupProvider) client() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return http.DefaultClient
}

func (p *CodeupProvider) matchesHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if strings.TrimSpace(p.domain) == "" {
		return strings.Contains(host, "codeup")
	}
	u, err := url.Parse(p.domain)
	if err != nil {
		return false
	}
	return strings.EqualFold(host, u.Hostname())
}

func normalizeCodeupDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return strings.TrimRight(raw, "/")
	}
	return "https://" + strings.TrimRight(raw, "/")
}

type codeupRemote struct {
	Host      string
	Namespace string
	Repo      string
}

func parseCodeupRemote(originURL string) (codeupRemote, error) {
	originURL = strings.TrimSpace(originURL)
	if originURL == "" {
		return codeupRemote{}, errors.New("empty origin url")
	}
	if strings.HasPrefix(originURL, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(originURL, "git@"), ":", 2)
		if len(parts) != 2 {
			return codeupRemote{}, errors.New("invalid ssh remote")
		}
		segments := splitCodeupPath(parts[1])
		if len(segments) < 2 {
			return codeupRemote{}, errors.New("invalid codeup path")
		}
		return codeupRemote{
			Host:      strings.ToLower(strings.TrimSpace(parts[0])),
			Namespace: strings.Join(segments[:len(segments)-1], "/"),
			Repo:      strings.TrimSuffix(segments[len(segments)-1], ".git"),
		}, nil
	}
	u, err := url.Parse(originURL)
	if err != nil {
		return codeupRemote{}, err
	}
	segments := splitCodeupPath(u.Path)
	if len(segments) < 2 {
		return codeupRemote{}, errors.New("invalid codeup path")
	}
	return codeupRemote{
		Host:      strings.ToLower(strings.TrimSpace(u.Hostname())),
		Namespace: strings.Join(segments[:len(segments)-1], "/"),
		Repo:      strings.TrimSuffix(segments[len(segments)-1], ".git"),
	}, nil
}

func splitCodeupPath(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func codeupOrganizationFromNamespace(namespace string) string {
	parts := splitCodeupPath(namespace)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func codeupRepositoryID(repo flowapp.ChangeRequestRepo) string {
	if repo.Namespace == "" || repo.Name == "" {
		return ""
	}
	return path.Join(repo.Namespace, repo.Name)
}

func codeupMergeMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "rebase":
		return "rebase"
	case "squash":
		return "squash"
	default:
		return "no-fast-forward"
	}
}

func parseCodeupChangeRequestList(raw json.RawMessage) ([]codeupChangeRequest, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var wrapped struct {
		Items []codeupChangeRequest `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Items != nil {
		return wrapped.Items, nil
	}

	var items []codeupChangeRequest
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	return nil, fmt.Errorf("decode codeup change request list failed: %s", strings.TrimSpace(string(raw)))
}

func stringSliceFromAny(v any) []string {
	switch vv := v.(type) {
	case []string:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if trimmed := strings.TrimSpace(stringFromAny(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		return nil
	}
}

func boolFromAny(v any) (bool, bool) {
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		switch strings.ToLower(strings.TrimSpace(vv)) {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		}
	}
	return false, false
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch vv := v.(type) {
	case string:
		return vv
	case fmt.Stringer:
		return vv.String()
	case json.Number:
		return vv.String()
	default:
		return fmt.Sprintf("%v", vv)
	}
}

func int64FromAny(v any) int64 {
	switch vv := v.(type) {
	case int:
		return int64(vv)
	case int64:
		return vv
	case int32:
		return int64(vv)
	case float64:
		return int64(vv)
	case float32:
		return int64(vv)
	case json.Number:
		n, _ := vv.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(vv), 10, 64)
		return n
	default:
		return 0
	}
}

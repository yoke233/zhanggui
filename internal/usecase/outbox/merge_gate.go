package outbox

import "context"

// CanMergeIssue checks whether an issue is ready for merge gate.
func (s *Service) CanMergeIssue(ctx context.Context, issueRef string) (bool, string, error) {
	issue, err := s.GetIssue(ctx, issueRef)
	if err != nil {
		return false, "", err
	}

	if containsString(issue.Labels, "needs-human") {
		return false, "needs-human present", nil
	}
	if !containsString(issue.Labels, "review:approved") {
		return false, "missing review:approved", nil
	}
	if !containsString(issue.Labels, "qa:pass") {
		return false, "missing qa:pass", nil
	}

	return true, "ready", nil
}

package outbox

import "errors"

var (
	ErrIssueRefRequired    = errors.New("issue ref is required")
	ErrInvalidIssueRef     = errors.New("invalid issue ref")
	ErrUnsupportedIssueRef = errors.New("unsupported issue ref")
	ErrInternalIssueIDRef  = errors.New("platform internal id cannot be used as issue ref")
	ErrInvalidStateLabel   = errors.New("invalid state label")

	ErrInvalidRunID          = errors.New("invalid run id")
	ErrRunIDRequired         = errors.New("run id is required")
	ErrInvalidResultCode     = errors.New("invalid result code")
	ErrWorkOrderMissingField = errors.New("work order missing required field")
	ErrWorkResultInvalid     = errors.New("work result is invalid")

	ErrIssueNotClaimed   = errors.New("work start requires assignee claim")
	ErrNeedsHuman        = errors.New("issue has needs-human label")
	ErrDependsUnresolved = errors.New("issue has unresolved dependencies")
	ErrTaskIssueBody     = errors.New("task issue body must include Goal and Acceptance Criteria")
)

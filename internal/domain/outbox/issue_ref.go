package outbox

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseIssueRef(issueRef string) (uint64, error) {
	trimmed := strings.TrimSpace(issueRef)
	if trimmed == "" {
		return 0, ErrIssueRefRequired
	}
	if !strings.HasPrefix(trimmed, "local#") {
		return 0, fmt.Errorf("%w: %q", ErrUnsupportedIssueRef, issueRef)
	}

	numText := strings.TrimPrefix(trimmed, "local#")
	if numText == "" {
		return 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	issueID, err := strconv.ParseUint(numText, 10, 64)
	if err != nil || issueID == 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}
	return issueID, nil
}

func FormatIssueRef(issueID uint64) string {
	return fmt.Sprintf("local#%d", issueID)
}

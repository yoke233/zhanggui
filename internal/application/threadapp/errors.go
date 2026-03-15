package threadapp

import "errors"

const (
	CodeCleanupThreadFailed  = "CLEANUP_THREAD_FAILED"
	CodeContextRefConflict   = "CONTEXT_REF_CONFLICT"
	CodeContextRefNotFound   = "CONTEXT_REF_NOT_FOUND"
	CodeInvalidContextAccess = "INVALID_CONTEXT_ACCESS"
	CodeLinkNotFound         = "LINK_NOT_FOUND"
	CodeMissingProjectID     = "MISSING_PROJECT_ID"
	CodeMissingTitle         = "MISSING_TITLE"
	CodeMissingWorkItemID    = "MISSING_WORK_ITEM_ID"
	CodeProjectNotFound      = "PROJECT_NOT_FOUND"
	CodeThreadNotFound       = "THREAD_NOT_FOUND"
	CodeWorkItemNotFound     = "WORK_ITEM_NOT_FOUND"
)

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code string, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func CodeOf(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

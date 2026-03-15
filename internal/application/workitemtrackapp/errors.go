package workitemtrackapp

import "errors"

const (
	CodeInvalidRelationType  = "INVALID_RELATION_TYPE"
	CodeInvalidState         = "INVALID_STATE"
	CodeMissingThreadID      = "MISSING_THREAD_ID"
	CodeMissingTitle         = "MISSING_TITLE"
	CodeThreadNotFound       = "THREAD_NOT_FOUND"
	CodeTrackNotFound        = "TRACK_NOT_FOUND"
	CodeWorkItemNotFound     = "WORK_ITEM_NOT_FOUND"
	CodeExecutionUnavailable = "EXECUTION_UNAVAILABLE"
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

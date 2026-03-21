package workitemapp

import "errors"

const (
	CodeBootstrapPRFailed          = "BOOTSTRAP_PR_FAILED"
	CodeInvalidResourceBinding     = "INVALID_RESOURCE_BINDING"
	CodeInvalidWorkItemDependency  = "INVALID_WORK_ITEM_DEPENDENCY"
	CodeInvalidState               = "INVALID_STATE"
	CodeMissingTitle               = "MISSING_TITLE"
	CodeNoActions                  = "NO_ACTIONS"
	CodeProjectNotFound            = "PROJECT_NOT_FOUND"
	CodeResourceBindingNotFound    = "RESOURCE_BINDING_NOT_FOUND"
	CodeWorkItemDependencyNotFound = "WORK_ITEM_DEPENDENCY_NOT_FOUND"
	CodeWorkItemNotFound           = "WORK_ITEM_NOT_FOUND"
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

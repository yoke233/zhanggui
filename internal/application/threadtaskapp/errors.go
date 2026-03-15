package threadtaskapp

import "errors"

const (
	CodeMissingThreadID  = "MISSING_THREAD_ID"
	CodeMissingTasks     = "MISSING_TASKS"
	CodeMissingAssignee  = "MISSING_ASSIGNEE"
	CodeMissingInstruction = "MISSING_INSTRUCTION"
	CodeInvalidTaskType  = "INVALID_TASK_TYPE"
	CodeInvalidAction    = "INVALID_ACTION"
	CodeInvalidState     = "INVALID_STATE"
	CodeThreadNotFound   = "THREAD_NOT_FOUND"
	CodeGroupNotFound    = "GROUP_NOT_FOUND"
	CodeTaskNotFound     = "TASK_NOT_FOUND"
	CodeRetryExhausted   = "RETRY_EXHAUSTED"
	CodeDependencyCycle  = "DEPENDENCY_CYCLE"
	CodeInvalidDependency = "INVALID_DEPENDENCY"
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

package gateway

import "fmt"

type DenyError struct {
	Code    string
	Message string
}

func (e DenyError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func deny(code, msg string) error {
	return DenyError{Code: code, Message: msg}
}

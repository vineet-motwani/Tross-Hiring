package errors

import "fmt"

type ProfileAPIError struct {
	Code    string
	Message string
}

func (e *ProfileAPIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

func NewProfileAPIError(code, message string) *ProfileAPIError {
	return &ProfileAPIError{
		Code:    code,
		Message: message,
	}
}

type InvalidProfileURLError struct {
	Message string
}

func (e *InvalidProfileURLError) Error() string {
	return e.Message
}

func (e *InvalidProfileURLError) Code() string {
	return "invalid_request"
}

package taskservice

import "fmt"

type SubmissionError struct {
	Code        string
	SafeMessage string
	cause       error
}

func newSubmissionError(code, safeMessage string, cause error) *SubmissionError {
	return &SubmissionError{Code: code, SafeMessage: safeMessage, cause: cause}
}

func (e *SubmissionError) Error() string {
	if e == nil {
		return "submission failed"
	}
	if e.cause == nil {
		return e.SafeMessage
	}
	return fmt.Sprintf("%s: %v", e.Code, e.cause)
}

func (e *SubmissionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

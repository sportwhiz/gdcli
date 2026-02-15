package errors

import (
	stderrors "errors"
	"fmt"
)

type Code string

const (
	CodeValidation   Code = "validation_error"
	CodeAuth         Code = "auth_error"
	CodeRateLimited  Code = "rate_limited"
	CodeProvider     Code = "provider_error"
	CodeBudget       Code = "budget_violation"
	CodeConfirmation Code = "confirmation_error"
	CodeSafety       Code = "safety_policy_violation"
	CodePartial      Code = "partial_failure"
	CodeInternal     Code = "internal_error"
)

type AppError struct {
	Code      Code           `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable"`
	DocURL    string         `json:"doc_url,omitempty"`
	Cause     error          `json:"-"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Cause }

func New(code Code, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

func Wrap(code Code, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, Cause: cause}
}

func WithDetails(err *AppError, details map[string]any) *AppError {
	err.Details = details
	return err
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var appErr *AppError
	if ok := As(err, &appErr); !ok {
		return 5
	}
	switch appErr.Code {
	case CodeValidation:
		return 2
	case CodeAuth:
		return 3
	case CodeRateLimited:
		return 4
	case CodeProvider, CodeInternal:
		return 5
	case CodeBudget:
		return 6
	case CodeConfirmation:
		return 7
	case CodeSafety:
		return 8
	case CodePartial:
		return 9
	default:
		return 5
	}
}

func As(err error, target **AppError) bool {
	if err == nil {
		return false
	}
	var t *AppError
	if !stderrors.As(err, &t) {
		return false
	}
	*target = t
	return true
}

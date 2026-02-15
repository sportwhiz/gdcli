package errors

import "testing"

func TestExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{&AppError{Code: CodeValidation}, 2},
		{&AppError{Code: CodeAuth}, 3},
		{&AppError{Code: CodeRateLimited}, 4},
		{&AppError{Code: CodeProvider}, 5},
		{&AppError{Code: CodeBudget}, 6},
		{&AppError{Code: CodeConfirmation}, 7},
		{&AppError{Code: CodeSafety}, 8},
		{&AppError{Code: CodePartial}, 9},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.code {
			t.Fatalf("expected %d got %d", c.code, got)
		}
	}
}

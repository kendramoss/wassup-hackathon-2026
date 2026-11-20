package domain

import (
	"fmt"
	"strings"
)

// ErrInvalid is wrapped by every validation error, so the HTTP layer can map
// any of them to 400 with errors.Is.
var ErrInvalid error = classedError{"invalid input", "invalid_input"}

// classedError names its own log class (a closed value read by the log
// sanitiser, #285); it carries no request text.
type classedError struct{ msg, class string }

func (e classedError) Error() string    { return e.msg }
func (e classedError) LogClass() string { return e.class }

// ValidationError lists every problem found in one value.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalid, strings.Join(e.Problems, "; "))
}

func (e *ValidationError) Unwrap() error { return ErrInvalid }

// problems collects validation messages; err returns nil when there are none.
type problems []string

func (p *problems) add(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Problems: p}
}

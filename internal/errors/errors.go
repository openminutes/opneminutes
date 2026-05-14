package errors

import (
	stderrors "errors"
	"fmt"
	"io/fs"
	"os"
)

// Kind identifies a high-level error category that callers can inspect without
// depending on error message text.
type Kind string

const (
	KindUnknown      Kind = ""
	KindConfig       Kind = "config"
	KindAuth         Kind = "auth"
	KindValidation   Kind = "validation"
	KindRemote       Kind = "remote"
	KindFileSystem   Kind = "file_system"
	KindConfirmation Kind = "confirmation"
)

// Error wraps an underlying error with a stable category.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}

	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// New creates a categorized error with the supplied message.
func New(kind Kind, message string) error {
	return &Error{Kind: kind, Err: stderrors.New(message)}
}

// Errorf creates a categorized error with a formatted message.
func Errorf(kind Kind, format string, args ...any) error {
	return &Error{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Wrap categorizes an existing error while preserving errors.Is/As behavior.
func Wrap(kind Kind, err error) error {
	if err == nil {
		return nil
	}

	return &Error{Kind: kind, Err: err}
}

// Wrapf categorizes an existing error and adds context while preserving
// errors.Is/As behavior.
func Wrapf(kind Kind, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}

	return &Error{Kind: kind, Err: fmt.Errorf(format+": %w", append(args, err)...)}
}

// KindOf returns the first categorized kind found in err's chain.
func KindOf(err error) Kind {
	var typed *Error
	if stderrors.As(err, &typed) {
		return typed.Kind
	}
	var pathErr *fs.PathError
	if stderrors.As(err, &pathErr) {
		return KindFileSystem
	}
	var linkErr *os.LinkError
	if stderrors.As(err, &linkErr) {
		return KindFileSystem
	}
	var syscallErr *os.SyscallError
	if stderrors.As(err, &syscallErr) {
		return KindFileSystem
	}

	return KindUnknown
}

// IsKind reports whether err has the requested category.
func IsKind(err error, kind Kind) bool {
	return KindOf(err) == kind
}

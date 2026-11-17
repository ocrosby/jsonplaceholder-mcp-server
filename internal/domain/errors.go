package domain

import (
	"errors"
	"fmt"
)

// Kind classifies a domain Error so adapters can map it without string matching.
type Kind int

// The error kinds.
const (
	// KindInvalid: the request was rejected before any upstream call.
	KindInvalid Kind = iota + 1
	// KindNotFound: the upstream API has no such resource.
	KindNotFound
	// KindUpstream: the upstream call failed or returned something unusable.
	KindUpstream
)

// Error is the single error type the application and ports speak.
type Error struct {
	Kind Kind
	// Status is the HTTP status behind the failure, or 0 when no response
	// was received (validation failure, connection failure, timeout).
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Invalid builds a KindInvalid error.
func Invalid(format string, a ...any) error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, a...)}
}

// NotFound builds a KindNotFound error with status 404.
func NotFound(format string, a ...any) error {
	return &Error{Kind: KindNotFound, Status: 404, Message: fmt.Sprintf(format, a...)}
}

// Upstream builds a KindUpstream error. Pass status 0 when there was no response.
func Upstream(status int, format string, a ...any) error {
	return &Error{Kind: KindUpstream, Status: status, Message: fmt.Sprintf(format, a...)}
}

// As reports whether err wraps a *Error and returns it.
func As(err error) (*Error, bool) {
	var de *Error
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

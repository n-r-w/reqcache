package reqcache

import "errors"

var (
	// ErrSessionAlreadyExists is returned when trying to create a session that already exists.
	ErrSessionAlreadyExists = errors.New("session already exists in context")

	// ErrNoSessionInContext is returned when there is no reqcache session in the context.
	ErrNoSessionInContext = errors.New("no reqcache session in context")
)

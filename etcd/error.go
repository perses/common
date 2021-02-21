package etcd

import "fmt"

const (
	ErrorCodeKeyConflict = 409
	ErrorCodeKeyNotFound = 404
)

// IsKeyNotFound returns true if the error code is ErrorCodeKeyNotFound.
func IsKeyNotFound(err error) bool {
	if cErr, ok := err.(*Error); ok {
		return cErr.Code == ErrorCodeKeyNotFound
	}
	return false
}

// IsKeyConflict returns true if the error code is ErrorCodeKeyConflict.
func IsKeyConflict(err error) bool {
	if cErr, ok := err.(*Error); ok {
		return cErr.Code == ErrorCodeKeyConflict
	}
	return false
}

type Error struct {
	Key  string
	Code int
}

func (e *Error) Error() string {
	return fmt.Sprintf("ErrorCode: %d, key: %s", e.Code, e.Key)
}

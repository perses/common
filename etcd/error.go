// Copyright 2021 Amadeus s.a.s
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

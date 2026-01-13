// Copyright The Perses Authors
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

package set

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
)

type Set[T comparable] map[T]struct{}

func New[T comparable](vals ...T) Set[T] {
	s := Set[T]{}
	for _, v := range vals {
		s[v] = struct{}{}
	}
	return s
}

// Merge merges two sets, giving priority to the old set in case of conflicts.
// It will create a new set and leave the input sets unmodified.
func Merge[T comparable](old, new Set[T]) Set[T] {
	if new == nil {
		return old
	}
	if old == nil {
		return new
	}
	s := Set[T]{}
	maps.Copy(s, new)
	maps.Copy(s, old)
	return s
}

func (s Set[T]) Add(vals ...T) {
	for _, v := range vals {
		s[v] = struct{}{}
	}
}

func (s Set[T]) Remove(value T) {
	delete(s, value)
}

func (s Set[T]) Contains(value T) bool {
	_, ok := s[value]
	return ok
}

// Merge adds all elements from the other set into the current set.
func (s Set[T]) Merge(other Set[T]) {
	for v := range other {
		s.Add(v)
	}
}

func (s Set[T]) TransformAsSlice() []T {
	if s == nil {
		return nil
	}

	var slice []T
	for v := range s {
		slice = append(slice, v)
	}
	slices.SortFunc(slice, compare[T])

	return slice
}

func (s Set[T]) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(s.TransformAsSlice())
}

func (s *Set[T]) UnmarshalJSON(b []byte) error {
	var slice []T
	if err := json.Unmarshal(b, &slice); err != nil {
		return err
	}
	if len(slice) == 0 {
		return nil
	}
	*s = make(map[T]struct{}, len(slice))
	for _, v := range slice {
		s.Add(v)
	}
	return nil
}

// compare has similar semantics to cmp.Compare except that it works for
// strings and structs. When comparing Go structs, it only checks the struct
// fields of string type.
// If the compared values aren't strings or structs, they are considered equal.
func compare[T comparable](a, b T) int {
	switch reflect.TypeOf(a).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmp.Compare(
			reflect.ValueOf(a).Int(),
			reflect.ValueOf(b).Int(),
		)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cmp.Compare(
			reflect.ValueOf(a).Uint(),
			reflect.ValueOf(b).Uint(),
		)
	case reflect.Float32, reflect.Float64:
		return cmp.Compare(
			reflect.ValueOf(a).Float(),
			reflect.ValueOf(b).Float(),
		)
	case reflect.String:
		return cmp.Compare(
			reflect.ValueOf(a).String(),
			reflect.ValueOf(b).String(),
		)
	case reflect.Struct:
		return cmp.Compare(
			buildKey(reflect.ValueOf(a)),
			buildKey(reflect.ValueOf(b)),
		)
	}

	return 0
}

func buildKey(v reflect.Value) string {
	var key strings.Builder
	for i := 0; i < v.Type().NumField(); i++ {
		v := v.Field(i)
		if v.Type().Kind() != reflect.String {
			continue
		}
		key.WriteString(v.String())
	}

	return key.String()
}

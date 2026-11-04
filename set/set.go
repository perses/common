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
	"fmt"
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
		return maps.Clone(old)
	}
	if old == nil {
		return maps.Clone(new)
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
	return s.setDecodedValues(slice)
}

// setDecodedValues replaces the content of the set with the values decoded from JSON or YAML.
// A decoded value may not be comparable (like a map decoded in a Set[any]), and using it as a key of the map would panic,
// so an error is returned instead.
func (s *Set[T]) setDecodedValues(values []T) error {
	if len(values) == 0 {
		return nil
	}
	for _, v := range values {
		if !reflect.ValueOf(&v).Elem().Comparable() {
			return fmt.Errorf("unable to add %v to the set: the value is not comparable", v)
		}
	}
	*s = make(map[T]struct{}, len(values))
	for _, v := range values {
		s.Add(v)
	}
	return nil
}

func (s Set[T]) MarshalYAML() (any, error) {
	if len(s) == 0 {
		return []T{}, nil
	}
	return s.TransformAsSlice(), nil
}

func (s *Set[T]) UnmarshalYAML(unmarshal func(any) error) error {
	var slice []T
	if err := unmarshal(&slice); err != nil {
		return err
	}
	return s.setDecodedValues(slice)
}

// compare has similar semantics to cmp.Compare except that it works for
// strings and structs. When comparing Go structs, it only checks the struct
// fields of string type.
// If the compared values aren't booleans, numbers, strings or structs, they are considered equal.
// When T is an interface, a nil value is placed first, and values of different types are ordered by type.
func compare[T comparable](a, b T) int {
	valueA := reflect.ValueOf(a)
	valueB := reflect.ValueOf(b)
	if !valueA.IsValid() || !valueB.IsValid() {
		return cmp.Compare(boolToInt(valueA.IsValid()), boolToInt(valueB.IsValid()))
	}
	if valueA.Type() != valueB.Type() {
		if c := cmp.Compare(valueA.Type().String(), valueB.Type().String()); c != 0 {
			return c
		}
		if c := cmp.Compare(valueA.Kind(), valueB.Kind()); c != 0 {
			return c
		}
	}
	switch valueA.Kind() {
	case reflect.Bool:
		return cmp.Compare(boolToInt(valueA.Bool()), boolToInt(valueB.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmp.Compare(valueA.Int(), valueB.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cmp.Compare(valueA.Uint(), valueB.Uint())
	case reflect.Float32, reflect.Float64:
		return cmp.Compare(valueA.Float(), valueB.Float())
	case reflect.String:
		return cmp.Compare(valueA.String(), valueB.String())
	case reflect.Struct:
		return cmp.Compare(buildKey(valueA), buildKey(valueB))
	}

	return 0
}

func boolToInt(b bool) int {
	if b {
		return 1
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

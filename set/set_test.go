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
	"encoding/json"
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	set := New(1, 2, 3)
	if len(set) != 3 {
		t.Errorf("Expected set length 3, got %d", len(set))
	}
	if !set.Contains(1) || !set.Contains(2) || !set.Contains(3) {
		t.Errorf("Set does not contain expected elements")
	}
}

func TestMerge(t *testing.T) {
	set1 := New(1, 2)
	set2 := New(3, 4)
	merged := Merge(set1, set2)
	if len(merged) != 4 {
		t.Errorf("Expected merged set length 4, got %d", len(merged))
	}
}

func TestSetAdd(t *testing.T) {
	set := New(1, 2)
	set.Add(3)
	if !set.Contains(3) {
		t.Errorf("Set does not contain added element")
	}
}

func TestSetRemove(t *testing.T) {
	set := New(1, 2, 3)
	set.Remove(2)
	if set.Contains(2) {
		t.Errorf("Set still contains removed element")
	}
}

func TestSetContains(t *testing.T) {
	set := New(1, 2, 3)
	if !set.Contains(2) {
		t.Errorf("Set does not contain expected element")
	}
	if set.Contains(4) {
		t.Errorf("Set contains unexpected element")
	}
}

func TestSetMerge(t *testing.T) {
	set1 := New(1, 2)
	set2 := New(3, 4)
	set1.Merge(set2)
	if len(set1) != 4 {
		t.Errorf("Expected merged set length 4, got %d", len(set1))
	}
}

func TestSetTransformAsSlice(t *testing.T) {
	set := New(3, 1, 2)
	slice := set.TransformAsSlice()
	if len(slice) != 3 {
		t.Errorf("Expected slice length 3, got %d", len(slice))
	}
	if !reflect.DeepEqual(slice, []int{1, 2, 3}) {
		t.Errorf("Expected sorted slice [1, 2, 3], got %v", slice)
	}
}

func TestSetMarshalJSON(t *testing.T) {
	set := New(1, 2, 3)
	data, err := json.Marshal(set)
	if err != nil {
		t.Errorf("Failed to marshal JSON: %v", err)
	}
	expected := "[1,2,3]"
	if string(data) != expected {
		t.Errorf("Expected JSON %s, got %s", expected, string(data))
	}
}

func TestSetUnmarshalJSON(t *testing.T) {
	data := "[1,2,3]"
	var set Set[int]
	err := json.Unmarshal([]byte(data), &set)
	if err != nil {
		t.Errorf("Failed to unmarshal JSON: %v", err)
	}
	if len(set) != 3 || !set.Contains(1) || !set.Contains(2) || !set.Contains(3) {
		t.Errorf("Set does not contain expected elements after unmarshalling")
	}
}

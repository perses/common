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

package slices

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvertSubContains(t *testing.T) {
	testSuites := []struct {
		title  string
		a      []string
		x      string
		result bool
	}{
		{
			title:  "empty array",
			result: false,
		},
		{
			title:  "not contain",
			a:      []string{"a", "b"},
			x:      "c",
			result: false,
		},
		{
			title:  "not contain 2",
			a:      []string{"/api/v1/projects", "b"},
			x:      "/api/v1/prometheus",
			result: false,
		},
		{
			title:  "contain",
			a:      []string{"a", "b"},
			x:      "a",
			result: true,
		},
		{
			title:  "contain 2",
			a:      []string{"/api/v1/projects", "/api/v1/prometheus"},
			x:      "/api/v1/projects/my_project",
			result: true,
		},
	}
	for _, test := range testSuites {
		t.Run(test.title, func(t *testing.T) {
			assert.Equal(t, test.result, InvertSubContains(test.a, test.x))
		})
	}
}

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

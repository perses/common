package slices

import "strings"

// InvertSubContains returns true if one of the string in a is contained in x
func InvertSubContains(a []string, x string) bool {
	for _, n := range a {
		if strings.Contains(x, n) {
			return true
		}
	}
	return false
}

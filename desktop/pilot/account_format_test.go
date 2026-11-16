package pilot

import (
	"strings"
	"testing"
)

func TestAccountIdentifierCanonicalBoundary(t *testing.T) {
	for _, value := range []string{strings.Repeat("0", 64), strings.Repeat("f", 64), strings.Repeat("ab", 32)} {
		if !validAccountID(value) {
			t.Fatal("canonical fixed-width identifier rejected")
		}
	}
	for _, value := range []string{"", strings.Repeat("0", 63), strings.Repeat("0", 65), strings.Repeat("AB", 32), strings.Repeat("g", 64), strings.Repeat("a", 1<<20)} {
		if validAccountID(value) {
			t.Fatal("invalid identifier accepted")
		}
	}
}

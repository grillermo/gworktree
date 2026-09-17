package main

import (
	"strings"
	"testing"
)

func TestTrimNameKeepsShortNamesVerbatim(t *testing.T) {
	for _, name := range []string{"", "sc-123", "feat/sc-1234-sweep-messages", "trailing-"} {
		if got := trimName(name); got != name {
			t.Errorf("trimName(%q) = %q, want it untouched", name, got)
		}
	}
}

func TestTrimNameCutsToFiftyCharacters(t *testing.T) {
	long := "feat/sc-1234-" + strings.Repeat("a", 80)
	got := trimName(long)
	if len([]rune(got)) != maxNameLen {
		t.Fatalf("trimName gave %d characters, want %d", len([]rune(got)), maxNameLen)
	}
	if !strings.HasPrefix(long, got) {
		t.Fatalf("trimName(%q) = %q, want the front kept", long, got)
	}
}

// A cut landing on a separator would leave 'feat/x-', which git will not accept
// as a ref name.
func TestTrimNameDropsSeparatorsLeftByTheCut(t *testing.T) {
	for _, sep := range []string{"-", "_", ".", "/"} {
		long := strings.Repeat("a", maxNameLen-1) + sep + strings.Repeat("b", 20)
		got := trimName(long)
		if strings.HasSuffix(got, sep) {
			t.Errorf("trimName left a trailing %q: %q", sep, got)
		}
	}
	// Several in a row all go, not just the last one.
	long := strings.Repeat("a", maxNameLen-3) + "-./" + strings.Repeat("b", 20)
	if got := trimName(long); strings.HasSuffix(got, "-") || strings.HasSuffix(got, ".") || strings.HasSuffix(got, "/") {
		t.Errorf("trimName left a separator run: %q", got)
	}
}

func TestZshQuoteSurvivesAQuoteInThePath(t *testing.T) {
	got := zshQuote("/Users/o'brien/c/gworktree/bin")
	want := `'/Users/o'\''brien/c/gworktree/bin'`
	if got != want {
		t.Fatalf("zshQuote gave %s, want %s", got, want)
	}
}

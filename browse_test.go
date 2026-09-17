package main

import (
	"strings"
	"testing"

	"github.com/grillermo/chicle"
)

func TestCreateNameTakesASingleTerm(t *testing.T) {
	for _, tc := range []struct {
		filter string
		want   string
		ok     bool
	}{
		{"sc-45621", "sc-45621", true},
		{"feat/sc-1/a-long-slug", "feat/sc-1/a-long-slug", true},
		{"  padded  ", "padded", true},
		{"", "", false},
		{"   ", "", false},
		{"sc-123 mail", "", false}, // two search terms, not one branch name
	} {
		got, ok := createName(tc.filter)
		if got != tc.want || ok != tc.ok {
			t.Errorf("createName(%q) = %q,%v; want %q,%v", tc.filter, got, ok, tc.want, tc.ok)
		}
	}
}

// createAction is the Create branch button, looked up by label so the test does
// not pin its position in the row.
func createAction(t *testing.T) chicle.Action {
	t.Helper()
	for _, a := range browseActions() {
		if a.Label == "Create branch" {
			return a
		}
	}
	t.Fatal("browse list has no Create branch action")
	return chicle.Action{}
}

func TestCreateIsOfferedOnlyForANameableFilter(t *testing.T) {
	show := createAction(t).Show
	for filter, want := range map[string]bool{
		"":            false, // nothing typed: nothing to name
		"sc-45621":    true,
		"sc-123 mail": false, // two terms
	} {
		if got := show(chicle.Selection{Filter: filter}); got != want {
			t.Errorf("Show(%q) = %v, want %v", filter, got, want)
		}
	}
}

func TestCreateRunsEvenWhenTheFilterMatchesNothing(t *testing.T) {
	if !createAction(t).OnEmpty {
		t.Fatal("Create branch must run on an empty list; that is the case it is for")
	}
}

func TestCreateEmitsTheCreateVerbAndTheTypedName(t *testing.T) {
	out := createAction(t).Run(chicle.Selection{Filter: "sc-45621"})
	if !out.Done {
		t.Fatal("Create branch did not end the picker")
	}
	verb, name, _ := strings.Cut(out.Result, "\t")
	if verb != "create" || name != "sc-45621" {
		t.Fatalf("got %q/%q, want create/sc-45621", verb, name)
	}
}

func TestCreateNamesTheBranchInItsConfirmation(t *testing.T) {
	if q := createAction(t).Confirm(chicle.Selection{Filter: "sc-45621"}); !strings.Contains(q, "sc-45621") {
		t.Fatalf("confirmation %q does not show the branch name", q)
	}
}

// The browse list's other actions must keep acting on the row, not the query.
func TestEnterAndRemoveStillActOnTheCursorRow(t *testing.T) {
	sel := chicle.Selection{Cursor: chicle.Row{Key: "feat/x"}, Filter: "unrelated"}
	for _, a := range browseActions() {
		switch a.Label {
		case "Enter", "Remove":
			if got := a.Run(sel).Result; !strings.HasSuffix(got, "\tfeat/x") {
				t.Errorf("%s produced %q, want it to act on feat/x", a.Label, got)
			}
			if a.OnEmpty {
				t.Errorf("%s runs on an empty list but needs a row", a.Label)
			}
		}
	}
}

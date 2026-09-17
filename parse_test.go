package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// porcelain is the shape `git worktree list --porcelain` emits: the primary
// working tree first, then one record per worktree, blank-line separated. The
// last one is prunable -- its directory was deleted from under git.
const porcelain = `worktree /repo
HEAD aaaa1111
branch refs/heads/main

worktree /repo/.worktrees/feat/sc-123
HEAD bbbb2222
branch refs/heads/feat/sc-123

worktree /repo/.worktrees/plain
HEAD cccc3333
detached

worktree /elsewhere/parked
HEAD dddd4444
branch refs/heads/parked

worktree /repo/.worktrees/gone
HEAD eeee5555
branch refs/heads/gone
prunable gitdir file points to non-existent location
`

func recordNames(rs []record) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.name)
	}
	return out
}

func TestParseKeepsOnlyLiveWorktreesUnderBase(t *testing.T) {
	got := recordNames(parseWorktrees(porcelain, filepath.FromSlash("/repo/.worktrees")))
	want := []string{filepath.FromSlash("feat/sc-123"), "plain"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseDropsAPrunableWorktree(t *testing.T) {
	for _, r := range parseWorktrees(porcelain, filepath.FromSlash("/repo/.worktrees")) {
		if r.name == "gone" {
			t.Fatal("a worktree whose directory is gone was offered anyway")
		}
	}
}

func TestParseKeepsPrunableScopedToItsOwnRecord(t *testing.T) {
	// The record after a prunable one must not inherit the flag.
	out := porcelain + `
worktree /repo/.worktrees/after
HEAD ffff6666
branch refs/heads/after
`
	got := recordNames(parseWorktrees(out, filepath.FromSlash("/repo/.worktrees")))
	if len(got) == 0 || got[len(got)-1] != "after" {
		t.Fatalf("got %v, want the record after a prunable one to survive", got)
	}
}

func TestParsePairsEachHeadWithItsOwnWorktree(t *testing.T) {
	for _, r := range parseWorktrees(porcelain, filepath.FromSlash("/repo/.worktrees")) {
		switch r.name {
		case filepath.FromSlash("feat/sc-123"):
			if r.head != "bbbb2222" {
				t.Errorf("feat/sc-123 got head %q, want bbbb2222", r.head)
			}
		case "plain":
			if r.head != "cccc3333" {
				t.Errorf("plain got head %q, want cccc3333", r.head)
			}
		}
	}
}

func TestParseIgnoresAWorktreeParkedOutsideBase(t *testing.T) {
	for _, r := range parseWorktrees(porcelain, filepath.FromSlash("/repo/.worktrees")) {
		if strings.Contains(r.name, "parked") || strings.HasPrefix(r.name, "..") {
			t.Fatalf("picked up a worktree outside .worktrees: %q", r.name)
		}
	}
}

func TestParseHandlesAnEmptyListing(t *testing.T) {
	if got := parseWorktrees("", filepath.FromSlash("/repo/.worktrees")); len(got) != 0 {
		t.Fatalf("got %v, want nothing", got)
	}
}

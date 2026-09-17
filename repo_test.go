package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo builds a throwaway repo on `main` with one commit, and points
// GWORKTREE_CD_FILE at a scratch file so the cd hand-off is exercised rather
// than falling back to the "no shell integration" message.
func newRepo(t *testing.T) string {
	t.Helper()

	root := resolve(t.TempDir())
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "init")

	t.Setenv("GWORKTREE_CD_FILE", filepath.Join(t.TempDir(), "cd"))
	return root
}

// cdDestination is what the shell wrapper would cd into after the last command.
func cdDestination(t *testing.T) string {
	t.Helper()
	out, err := os.ReadFile(os.Getenv("GWORKTREE_CD_FILE"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestCreateMakesTheWorktreeAndPointsTheShellAtIt(t *testing.T) {
	root := newRepo(t)

	if err := create(root, "feat/sc-123"); err != nil {
		t.Fatal(err)
	}

	path := worktreePath(root, "feat/sc-123")
	if !isDir(path) {
		t.Fatalf("no worktree at %s", path)
	}
	if got := cdDestination(t); got != path {
		t.Fatalf("shell was sent to %q, want %q", got, path)
	}
	if got, _ := names(root); len(got) != 1 || got[0] != "feat/sc-123" {
		t.Fatalf("listing is %v, want [feat/sc-123]", got)
	}
}

func TestCreateOnAnExistingWorktreeJustEntersIt(t *testing.T) {
	root := newRepo(t)
	if err := create(root, "reuse"); err != nil {
		t.Fatal(err)
	}
	if err := create(root, "reuse"); err != nil {
		t.Fatalf("second create failed instead of entering: %v", err)
	}
	if got := cdDestination(t); got != worktreePath(root, "reuse") {
		t.Fatalf("shell was sent to %q, want the existing worktree", got)
	}
}

// An unmerged branch is the case the safety rail exists for: nothing is
// removed, and the message names the way through.
func TestRemoveRefusesAnUnmergedBranch(t *testing.T) {
	root := newRepo(t)
	if err := create(root, "unmerged"); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "unmerged")
	commitIn(t, path, "work")

	err := removeOne(root, "main", "unmerged", false)
	if err == nil {
		t.Fatal("removed an unmerged worktree")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("message does not point at --force: %v", err)
	}
	if !isDir(path) {
		t.Fatal("the worktree was removed anyway")
	}
}

func TestRemoveForceTakesAnUnmergedBranchAndPrunesItsParent(t *testing.T) {
	root := newRepo(t)
	if err := create(root, "feat/gone"); err != nil {
		t.Fatal(err)
	}
	commitIn(t, worktreePath(root, "feat/gone"), "work")

	if err := removeOne(root, "main", "feat/gone", true); err != nil {
		t.Fatal(err)
	}
	if isDir(worktreePath(root, "feat/gone")) {
		t.Fatal("the worktree is still there")
	}
	// .worktrees/feat only existed to hold it.
	if isDir(filepath.Join(worktreesDir(root), "feat")) {
		t.Fatal("the empty parent directory was left behind")
	}
	if gitOK(root, "show-ref", "--verify", "--quiet", "refs/heads/feat/gone") {
		t.Fatal("the branch survived a force removal")
	}
}

func TestRemoveTakesAMergedBranch(t *testing.T) {
	root := newRepo(t)
	if err := create(root, "merged"); err != nil {
		t.Fatal(err)
	}
	// Never diverged from main, so it counts as merged.
	if err := removeOne(root, "main", "merged", false); err != nil {
		t.Fatal(err)
	}
	if isDir(worktreePath(root, "merged")) {
		t.Fatal("the worktree is still there")
	}
	if got, _ := names(root); len(got) != 0 {
		t.Fatalf("listing is %v, want nothing", got)
	}
}

func TestMatchingIsCaseInsensitiveAndPartial(t *testing.T) {
	root := newRepo(t)
	for _, name := range []string{"feat/SC-45621-sweep", "chore/tidy"} {
		if err := create(root, name); err != nil {
			t.Fatal(err)
		}
	}
	got, err := matching(root, "sc-456")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "feat/SC-45621-sweep" {
		t.Fatalf("matching gave %v, want just the SC-45621 worktree", got)
	}
}

// commitIn puts a commit on the branch checked out at path, so it no longer
// counts as merged into the base branch.
func commitIn(t *testing.T, path, file string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, file), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", file}, {"commit", "-m", "work"}} {
		full := append([]string{"-C", path, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

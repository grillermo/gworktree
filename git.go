package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// gitOut runs git in root and returns its trimmed stdout. git's own stderr is
// swallowed: every caller either has a better message for the failure or treats
// it as a plain "no".
func gitOut(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// gitRun runs git with its output left on the terminal, for the commands whose
// progress and errors the user should see verbatim.
func gitRun(root string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// gitOK reports whether git succeeded, for the checks whose whole answer is the
// exit status.
func gitOK(root string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", root}, args...)...).Run() == nil
}

// repoRoot returns the *primary* working tree, not whichever worktree we happen
// to be standing in: --show-toplevel would give us .worktrees/<name> and every
// path below would nest another level deeper. --git-common-dir points at the
// primary repo's .git even from inside a worktree, so its parent is the main
// checkout.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.New("not in a git repository")
		}
		return "", err
	}
	return filepath.Dir(strings.TrimSpace(string(out))), nil
}

// worktreesDir is where gworktree keeps every checkout it manages.
func worktreesDir(root string) string { return filepath.Join(root, ".worktrees") }

func worktreePath(root, name string) string {
	return filepath.Join(worktreesDir(root), filepath.FromSlash(name))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// maxNameLen caps a branch name. A long ticket slug can be pasted in as-is and
// is cut to fit rather than rejected.
const maxNameLen = 50

// trimName cuts a name to maxNameLen characters, keeping the front. Trailing
// separators left by the cut are dropped too: 'feat/x-' is not a valid ref
// name, and neither `git branch` nor a directory path wants to end on one.
func trimName(name string) string {
	if utf8.RuneCountInString(name) <= maxNameLen {
		return name
	}
	runes := []rune(name)[:maxNameLen]
	for len(runes) > 0 && strings.ContainsRune("-_./", runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

// remoteFor returns the first remote that has refs/remotes/<remote>/<name>,
// preferring origin, then github, then whatever else is configured.
func remoteFor(root, name string) (string, bool) {
	out, err := gitOut(root, "remote")
	if err != nil {
		return "", false
	}
	for _, r := range append([]string{"origin", "github"}, strings.Fields(out)...) {
		if gitOK(root, "show-ref", "--verify", "--quiet", "refs/remotes/"+r+"/"+name) {
			return r, true
		}
	}
	return "", false
}

// warnIfBehind reports (without touching the network) that the branch checked
// out at path has fallen behind its upstream. gworktree never updates a
// worktree, so entering a stale one silently would be a trap.
func warnIfBehind(path string) {
	counts, err := gitOut(path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		return
	}
	// "<behind>\t<ahead>"; the left side is what we are missing.
	fields := strings.Fields(counts)
	if len(fields) == 0 || fields[0] == "0" {
		return
	}
	behind := fields[0]
	upstream, _ := gitOut(path, "rev-parse", "--abbrev-ref", "@{upstream}")
	fmt.Fprintf(os.Stderr, "⚠️  %s commit(s) behind %s as of your last fetch.\n", behind, upstream)
	fmt.Fprintf(os.Stderr, "    Update with: git -C '%s' pull --ff-only\n", path)
}

// pruneEmptyDirs drops the directories a nested name leaves behind: a branch
// like feat/sc-123 leaves .worktrees/feat once its worktree is gone. Only empty
// ones go, and only up to .worktrees itself.
func pruneEmptyDirs(base, dir string) {
	for dir != base && strings.HasPrefix(dir, base) && isDir(dir) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// ensureBase resolves the branch worktrees are merged back into -- master when
// it exists, then main -- and makes sure the primary working tree is on it.
// That is what "merged" is measured against, and what the worktrees are
// removed from.
func ensureBase(root string) (string, error) {
	base, err := baseBranch(root)
	if err != nil {
		return "", err
	}
	current, err := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if current != base {
		if err := gitRun(root, "checkout", base); err != nil {
			return "", err
		}
	}
	return base, nil
}

// isMerged reports whether a local branch has already landed on base.
func isMerged(root, base, name string) bool {
	out, err := gitOut(root, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

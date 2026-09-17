package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// removeCmd backs `gworktree remove [--force] <name>...`. Each argument is one
// worktree: a failure on one does not stop the rest, and the exit status
// reports whether anything failed.
func removeCmd(root string, args []string) error {
	force := false
	var names []string
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		default:
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		return errors.New("usage: gworktree remove [--force] <name> [<name>...]")
	}

	base, err := ensureBase(root)
	if err != nil {
		return err
	}

	failed := 0
	for _, name := range names {
		if err := removeOne(root, base, name, force); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed++
		}
	}
	return someFailed(failed)
}

// removeOne removes a single worktree plus its branch, provided the branch is
// merged into base. It assumes the primary working tree is already on that base
// branch. With force set, the worktree goes even if it has modified or
// untracked files and the branch goes even if it never landed.
func removeOne(root, base, raw string, force bool) error {
	name := trimName(raw)
	path := worktreePath(root, name)
	if !isDir(path) {
		return fmt.Errorf("worktree %q not found at %s", name, path)
	}

	stepOutOf(root, path)

	merged := isMerged(root, base, name)

	if force {
		// Drop the worktree along with any uncommitted work in it, and delete
		// the branch whether or not it ever landed on base.
		if err := gitRun(root, "worktree", "remove", "--force", path); err != nil {
			return err
		}
		if err := gitRun(root, "branch", "-D", name); err != nil {
			return err
		}
		pruneEmptyDirs(worktreesDir(root), filepath.Dir(path))
		if merged {
			fmt.Printf("✅ Removed worktree and branch %q.\n", name)
		} else {
			fmt.Printf("✅ Force-removed worktree and unmerged branch %q.\n", name)
		}
		return nil
	}

	if !merged {
		return fmt.Errorf("⚠️  Branch %q is not merged into %s; leaving the worktree in place.\n"+
			"    Force removal with: gworktree remove --force '%s'", name, base, name)
	}

	if err := gitRun(root, "worktree", "remove", path); err != nil {
		return fmt.Errorf("%w\n    Retry with: gworktree remove --force '%s'", err, name)
	}
	if err := gitRun(root, "branch", "-d", name); err != nil {
		return err
	}
	pruneEmptyDirs(worktreesDir(root), filepath.Dir(path))
	fmt.Printf("✅ Removed worktree and branch %q.\n", name)
	return nil
}

// stepOutOf moves the caller back to the repo root when it is standing in the
// worktree about to be removed -- otherwise the shell is left in a directory
// that no longer exists. Both sides are resolved first: the shell's $PWD may be
// a symlinked path (/tmp) where git reports the real one (/private/tmp).
func stepOutOf(root, path string) {
	pwd, err := os.Getwd()
	if err != nil {
		return
	}
	inside := resolve(pwd)
	target := resolve(path)
	if inside == target || strings.HasPrefix(inside, target+string(filepath.Separator)) {
		cdTo(root)
	}
}

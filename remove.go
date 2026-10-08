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
//
// Either half may already be gone -- a folder deleted by hand, a branch deleted
// with git -- and whatever is left is still removed. Only a name with neither a
// folder nor a branch is an error, since that is most likely a typo.
func removeOne(root, base, raw string, force bool) error {
	name := trimName(raw)
	path := worktreePath(root, name)
	hasDir := isDir(path)
	hasBranch := gitOK(root, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if !hasDir && !hasBranch {
		return fmt.Errorf("worktree %q not found: no folder at %s and no branch", name, path)
	}

	if hasDir {
		stepOutOf(root, path)
	} else {
		// A folder deleted by hand leaves git's record of the worktree behind,
		// and that record keeps the branch checked out, so git would refuse to
		// delete it.
		gitOK(root, "worktree", "prune")
	}

	// Only a branch can hold unmerged work; with it gone there is nothing for
	// the merge check to protect.
	merged := !hasBranch || isMerged(root, base, name)
	if !force && !merged {
		return fmt.Errorf("⚠️  Branch %q is not merged into %s; leaving the worktree in place.\n"+
			"    Force removal with: gworktree remove --force '%s'", name, base, name)
	}

	if hasDir {
		if err := removeDir(root, path, force); err != nil {
			if force {
				return err
			}
			return fmt.Errorf("%w\n    Retry with: gworktree remove --force '%s'", err, name)
		}
	}
	if hasBranch {
		// -D drops the branch whether or not it ever landed on base.
		flag := "-d"
		if force {
			flag = "-D"
		}
		if err := gitRun(root, "branch", flag, name); err != nil {
			return err
		}
	}
	pruneEmptyDirs(worktreesDir(root), filepath.Dir(path))

	switch {
	case !hasDir:
		fmt.Printf("✅ Removed branch %q; its worktree folder was already gone.\n", name)
	case !hasBranch:
		fmt.Printf("✅ Removed worktree %q; its branch was already gone.\n", name)
	case !merged:
		fmt.Printf("✅ Force-removed worktree and unmerged branch %q.\n", name)
	default:
		fmt.Printf("✅ Removed worktree and branch %q.\n", name)
	}
	return nil
}

// removeDir removes the worktree checked out at path. With force set, a folder
// git no longer recognises as a worktree -- its .git link broken, say -- is
// deleted outright rather than left behind: it sits under .worktrees, which
// gworktree owns.
func removeDir(root, path string, force bool) error {
	if !force {
		return gitRun(root, "worktree", "remove", path)
	}
	// Drop the worktree along with any uncommitted work in it.
	if gitOK(root, "worktree", "remove", "--force", path) {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	gitOK(root, "worktree", "prune")
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

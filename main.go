// Command gworktree manages git worktrees under .worktrees/<name>.
//
//	gworktree                   pick a worktree from a list, then enter or remove it
//	gworktree <name>            create (or jump into) .worktrees/<name> on branch <name>
//	gworktree <fragment>        when <fragment> is not a worktree but is part of the
//	                            name of one, open the list pre-filtered to it instead
//	gworktree remove <name>...  checkout master/main, then remove each worktree whose branch is merged
//	gworktree remove --force <name>...  remove regardless of uncommitted work or merge state
//	gworktree clean             list every worktree, merged ones pre-ticked, and remove what you confirm
//	                            (`cleanup` is accepted too)
//
// <name> is used verbatim as the branch name, so it must already be a valid git
// ref:  gworktree feat/sc-1234-sweep-messages  ->  branch feat/sc-1234-sweep-messages
// (a name with slashes simply nests under .worktrees/). The only edit made is a
// cut to 50 characters, so a long ticket slug can be pasted in as-is.
//
// When <name> is not a local branch but IS a branch on a remote, the worktree is
// created from that remote branch with upstream tracking -- never as a fresh
// branch off whatever HEAD happens to be. gworktree does not fetch; run
// `git fetch` yourself if you need the remote-tracking refs refreshed.
//
// A process cannot change its parent shell's directory, so entering a worktree
// is handed back to the shell wrapper: see shell.go and `gworktree --shell-init`.
package main

import (
	"errors"
	"fmt"
	"os"
)

const usage = `Usage:
  gworktree                             pick a worktree, then enter or remove it
  gworktree <name>                      create or jump into .worktrees/<name>
  gworktree remove [--force] <name>...  remove merged worktrees (or any, with --force)
  gworktree clean                       tick worktrees to remove, merged ones pre-ticked
  gworktree --list                      print worktree names, one per line
  gworktree --shell-init zsh            print the shell integration to eval`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gworktree:", err)
		os.Exit(1)
	}
}

// run dispatches one invocation. Only the three subcommand words the shell
// function always reserved -- remove, clean, cleanup -- are unavailable as
// worktree names; everything else added here is a flag, which git refs cannot
// start with anyway.
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "--shell-init":
			return shellInit(args[1:])
		case "-h", "--help":
			fmt.Println(usage)
			return nil
		}
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return browse(root, "")
	}

	switch args[0] {
	case "--list":
		return listNames(root)
	case "remove":
		return removeCmd(root, args[1:])
	case "clean", "cleanup":
		if len(args) > 1 {
			return errors.New("usage: gworktree clean")
		}
		return cleanCmd(root)
	}

	if len(args) > 1 || args[0] == "" {
		return errors.New(usage)
	}
	return openOrCreate(root, args[0])
}

// openOrCreate is the `gworktree <name>` path: an exact worktree is never
// ambiguous, so go straight in. Otherwise, before treating the name as
// something to create, see whether it is a fragment of worktrees that already
// exist -- `gworktree sc-45621` for the one whose full name merely contains it.
// Those go to the picker with the fragment pre-filled rather than being guessed
// at, since picking wrong here means a stray branch. A fragment matching
// nothing still creates silently: that is the common case and does not need a
// keystroke.
//
// Diverting needs somewhere to divert to: with no terminal there is nobody to
// answer the question, and silently listing instead of creating would break
// `gworktree <name>` in a script.
func openOrCreate(root, arg string) error {
	name := trimName(arg)

	if path := worktreePath(root, name); isDir(path) {
		return enter(path)
	}

	near, err := matching(root, name)
	if err == nil && len(near) > 0 && interactive() {
		return browse(root, name)
	}
	return create(root, arg)
}

// listNames backs shell completion: the worktree names, one per line.
func listNames(root string) error {
	all, err := names(root)
	if err != nil {
		return err
	}
	for _, n := range all {
		fmt.Println(n)
	}
	return nil
}

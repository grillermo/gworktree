package main

import (
	"fmt"
	"os"
)

// create makes the worktree for name and enters it: reuse a local branch, track
// a remote one, or branch from HEAD -- in that order, so a name someone else
// already pushed never becomes an unrelated new branch off whatever HEAD is.
func create(root, raw string) error {
	name := trimName(raw)
	if name != raw {
		fmt.Println("Branch name:", name)
	}

	path := worktreePath(root, name)

	// Already exists -> just jump into it.
	if isDir(path) {
		return enter(path)
	}

	switch {
	case gitOK(root, "show-ref", "--verify", "--quiet", "refs/heads/"+name):
		// Local branch already exists -> reuse it as-is.
		if err := gitRun(root, "worktree", "add", path, name); err != nil {
			return err
		}
	default:
		remote, onRemote := remoteFor(root, name)
		if onRemote {
			// Someone else's branch: base it on the remote and track it, so
			// that committing and plain `git push` work without further setup.
			fmt.Printf("Found %s/%s -> creating tracking branch %q.\n", remote, name, name)
			if err := gitRun(root, "worktree", "add", "--track", "-b", name, path, remote+"/"+name); err != nil {
				return err
			}
			break
		}
		// Genuinely new work -> branch from current HEAD.
		head, _ := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
		fmt.Printf("No local or remote branch %q -> creating a new branch from %s.\n", name, head)
		if err := gitRun(root, "worktree", "add", "-b", name, path); err != nil {
			return err
		}
	}

	return enter(path)
}

// enter hands the destination back to the shell wrapper, which is the only
// thing that can actually change the caller's directory. gworktree never
// updates a worktree, so a checkout that has fallen behind is flagged on the
// way in.
func enter(path string) error {
	warnIfBehind(path)
	return cdTo(path)
}

// cdTo writes the directory the shell should move to. The wrapper hands us a
// scratch file for it; run straight off PATH there is no wrapper, so say what
// would have happened rather than silently doing nothing.
func cdTo(path string) error {
	file := os.Getenv("GWORKTREE_CD_FILE")
	if file == "" {
		fmt.Fprintf(os.Stderr, "gworktree: shell integration not loaded, so I cannot cd for you:\n    cd %s\n", path)
		return nil
	}
	return os.WriteFile(file, []byte(path), 0o600)
}

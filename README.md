# gworktree

Manage git worktrees under `.worktrees/<name>`.

```
gworktree                             pick a worktree, then enter or remove it
gworktree <name>                      create or jump into .worktrees/<name>
gworktree <fragment>                  open the list pre-filtered to <fragment>
gworktree remove [--force] <name>...  remove merged worktrees (or any, with --force)
gworktree clean                       tick worktrees to remove, merged ones pre-ticked
```

`<name>` is used verbatim as the branch name, so it must already be a valid git
ref: `gworktree feat/sc-1234-sweep-messages` puts a worktree on branch
`feat/sc-1234-sweep-messages` (a name with slashes simply nests under
`.worktrees/`). The only edit made is a cut to 50 characters, so a long ticket
slug can be pasted in as-is.

When `<name>` is not a local branch but *is* a branch on a remote, the worktree
is created from that remote branch with upstream tracking — never as a fresh
branch off whatever HEAD happens to be. gworktree does not fetch; run
`git fetch` yourself if you need the remote-tracking refs refreshed.

Removal only takes branches already merged into `master`/`main`, unless you pass
`--force`. Removing from the browse menu always forces, after a confirm. If the
worktree folder or the branch is already gone, whichever is left is still removed.

## Install

```bash
./build
```

Then one line in your zsh startup:

```zsh
eval "$(~/c/gworktree/bin/gworktree --shell-init zsh)"
```

That line is the whole install: it puts `bin/` on `PATH` and defines the wrapper
and the completion.

## Why the wrapper

A process cannot change its parent shell's directory, so entering a worktree has
to be handed back to the shell. The binary writes the destination to the scratch
file named by `$GWORKTREE_CD_FILE` and the wrapper does the `cd`. Run straight
off `PATH` with no wrapper, gworktree still does all the git work and just tells
you where to `cd`.

## Layout

- `main.go` — argument dispatch, and the `gworktree <name>` "enter, divert or
  create" decision.
- `git.go` — the git helpers every command shares: repo root, base branch, name
  trimming, the behind-upstream warning.
- `worktree.go` — reading `git worktree list --porcelain`, and the two slow
  columns (merged, on remote).
- `live.go` — those two columns answered in parallel, folded in as they land.
- `browse.go` / `clean.go` — the two [chicle](../chicle) lists.
- `create.go` / `remove.go` — the git work each verb does.
- `shell/gworktree.zsh` — the integration, embedded into the binary.

The binary is git-ignored, so `./build` only affects the local checkout.
Source changes do nothing until it is rerun.

package main

// The two interesting columns are both too slow to block the first draw:
// MERGED makes git walk history, and ON REMOTE is a network round trip. They
// are asked for in parallel and folded into the list as they land.

// which column an update carries, so the clean list can pre-tick exactly once,
// on the update that first knows what is merged.
type column int

const (
	colMerged column = iota
	colRemote
)

// stateUpdate is one background column landing: the whole worktree slice with
// that column filled in.
type stateUpdate struct {
	worktrees []worktree
	column    column
}

// liveStates answers both slow questions at once and emits a snapshot after
// each answer, so whichever finishes first shows up first. The channel closes
// when both are in.
func liveStates(root string, worktrees []worktree) <-chan stateUpdate {
	out := make(chan stateUpdate, 2)

	merged := lookup(func() (map[string]bool, error) { return mergedBranches(root) })
	remote := lookup(func() (map[string]bool, error) { return remoteBranches(root) })

	go func() {
		defer close(out)
		current := worktrees
		// A nil channel is never ready in a select, so each answer is taken
		// exactly once and the loop ends when both have been.
		for merged != nil || remote != nil {
			select {
			case b := <-merged:
				merged = nil
				current = withState(current, b, func(w *worktree, s state) { w.merged = s })
				out <- stateUpdate{worktrees: current, column: colMerged}
			case b := <-remote:
				remote = nil
				current = withState(current, b, func(w *worktree, s state) { w.onRemote = s })
				out <- stateUpdate{worktrees: current, column: colRemote}
			}
		}
	}()
	return out
}

// lookup runs one branch-set query off the drawing path.
func lookup(f func() (map[string]bool, error)) chan branchSet {
	ch := make(chan branchSet, 1)
	go func() {
		names, err := f()
		ch <- branchSet{names: names, err: err}
	}()
	return ch
}

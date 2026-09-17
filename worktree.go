package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// worktree is one checkout under .worktrees/: the name is the path relative to
// .worktrees (so it may contain slashes) and is also the branch name for
// worktrees gworktree created.
type worktree struct {
	name       string
	lastCommit time.Time // zero value means we could not read it
	merged     state
	onRemote   state
}

// state is what we know about one yes/no property of a worktree's branch. Both
// of the properties the list shows -- merged into the base branch, present on
// the remote -- start out pending: they are answered in the background.
type state int

const (
	statePending state = iota
	stateYes
	stateNo
	stateUnknown // the question could not be answered (no base branch, offline)
)

// label renders a state into its column cell.
func (s state) label() string {
	switch s {
	case stateYes:
		return "yes"
	case stateNo:
		return "no"
	case stateUnknown:
		return "?"
	default:
		return "..."
	}
}

// lastCommitLabel renders the timestamp column.
func (w worktree) lastCommitLabel() string {
	if w.lastCommit.IsZero() {
		return "unknown"
	}
	return w.lastCommit.Format("2006-01-02 15:04")
}

// listRecords returns the live worktrees under <root>/.worktrees, in the order
// git reports them.
//
// This asks git rather than globbing for **/.git. The glob has to descend every
// directory inside every worktree -- node_modules, tmp, vendor, git's own
// internals -- to find a handful of .git entries: on a large repo with a few
// dozen worktrees that is over 13 seconds, against 0.08 for git.
func listRecords(root string) ([]record, error) {
	out, err := gitOut(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out, worktreesDir(root)), nil
}

// names is the cheap listing: just the worktree names, sorted. It skips the
// per-worktree `git show` that dating the list needs, because the callers that
// want it -- shell completion, the no-terminal fallback -- have no columns to
// fill.
func names(root string) ([]string, error) {
	records, err := listRecords(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, filepath.ToSlash(r.name))
	}
	sort.Strings(out)
	return out, nil
}

// matching returns the worktree names containing needle. Case-insensitive and a
// fixed string, matching how the picker's own filter reads what we hand it.
func matching(root, needle string) ([]string, error) {
	all, err := names(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range all {
		if strings.Contains(strings.ToLower(n), strings.ToLower(needle)) {
			out = append(out, n)
		}
	}
	return out, nil
}

// loadWorktrees returns the worktrees under <root>/.worktrees, most recently
// committed to first. The primary working tree and any worktree parked
// elsewhere are left out: gworktree only manages the .worktrees ones.
func loadWorktrees(root string) ([]worktree, error) {
	records, err := listRecords(root)
	if err != nil {
		return nil, err
	}

	worktrees := make([]worktree, 0, len(records))
	for _, r := range records {
		worktrees = append(worktrees, worktree{
			name:       filepath.ToSlash(r.name),
			lastCommit: commitTime(root, r.head),
		})
	}

	sort.SliceStable(worktrees, func(i, j int) bool {
		return worktrees[i].lastCommit.After(worktrees[j].lastCommit)
	})
	return worktrees, nil
}

// record is one entry of `git worktree list --porcelain`, reduced to what the
// list needs: the name relative to .worktrees, and the revision to date.
type record struct {
	name string
	head string
}

// parseWorktrees reads the porcelain listing, keeping only the worktrees under
// base. Records run until the next "worktree " line.
//
// Prunable records are dropped. Those are worktrees whose directory has been
// deleted from under git: still registered, so still listed, but with nothing
// left to enter or remove. Offering one only produces a confusing failure when
// the user picks it.
func parseWorktrees(out, base string) []record {
	var records []record
	var current record
	var prunable bool

	flush := func() {
		if current.name != "" && !prunable {
			records = append(records, current)
		}
	}

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current, prunable = record{}, false
			if rel, ok := under(base, strings.TrimPrefix(line, "worktree ")); ok {
				current.name = rel
			}
		case strings.HasPrefix(line, "HEAD "):
			current.head = strings.TrimPrefix(line, "HEAD ")
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			prunable = true
		}
	}
	flush()
	return records
}

// under reports whether path sits inside base, and with what relative name.
// Both sides are resolved first: git reports the real path (/private/tmp)
// where the repo may be reached through a symlink (/tmp).
func under(base, path string) (string, bool) {
	base = resolve(base)
	rel, err := filepath.Rel(base, resolve(path))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

func resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// branchSet is the answer to "which branches are in this set", or the reason
// there is no answer. A failed lookup is not an empty set: it leaves the column
// unknown rather than telling the user "no" about every row.
type branchSet struct {
	names map[string]bool
	err   error
}

// withState fills one column in for every worktree from a branchSet. It returns
// a fresh slice: the caller is showing the old one while this runs.
func withState(worktrees []worktree, b branchSet, set func(*worktree, state)) []worktree {
	out := make([]worktree, len(worktrees))
	copy(out, worktrees)
	for i := range out {
		switch {
		case b.err != nil:
			set(&out[i], stateUnknown)
		case b.names[out[i].name]:
			set(&out[i], stateYes)
		default:
			set(&out[i], stateNo)
		}
	}
	return out
}

// mergedBranches lists the local branches already merged into the base branch.
// This is the slow part of the list -- git walks history for it -- so it is
// asked for once, off the path that draws the picker.
func mergedBranches(root string) (map[string]bool, error) {
	base, err := baseBranch(root)
	if err != nil {
		return nil, err
	}
	out, err := gitOut(root, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	merged := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			merged[name] = true
		}
	}
	return merged, nil
}

// remoteTimeout caps the one network call the list makes. A remote that is slow
// or unreachable must not hold the column pending forever -- past this the
// answer is "?" and the list carries on.
const remoteTimeout = 10 * time.Second

// remoteBranches asks the remote which branches it has *right now*. This is
// deliberately a network call rather than a read of refs/remotes/*: the whole
// point of the column is to say whether the branch still exists on GitHub, and
// a merged-and-deleted branch keeps its stale remote-tracking ref until the
// next fetch --prune, which is exactly the case the column exists to catch.
func remoteBranches(root string) (map[string]bool, error) {
	remote, err := primaryRemote(root)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-remote", "--heads", remote).Output()
	if err != nil {
		return nil, err
	}

	// Each line is "<sha>\trefs/heads/<branch>", and <branch> may contain
	// slashes, so split off the ref prefix rather than the last path element.
	branches := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		if name := strings.TrimPrefix(ref, "refs/heads/"); name != ref && name != "" {
			branches[name] = true
		}
	}
	return branches, nil
}

// primaryRemote picks the remote the ON REMOTE column speaks for, in the same
// order remoteFor prefers when it looks a single branch up.
func primaryRemote(root string) (string, error) {
	out, err := gitOut(root, "remote")
	if err != nil {
		return "", err
	}
	configured := strings.Fields(out)
	if len(configured) == 0 {
		return "", errors.New("no remotes configured")
	}
	for _, preferred := range []string{"origin", "github"} {
		for _, r := range configured {
			if r == preferred {
				return r, nil
			}
		}
	}
	return configured[0], nil
}

// baseBranch picks the branch worktrees are merged back into: master when it
// exists, then main.
func baseBranch(root string) (string, error) {
	for _, base := range []string{"master", "main"} {
		if gitOK(root, "show-ref", "--verify", "--quiet", "refs/heads/"+base) {
			return base, nil
		}
	}
	return "", errors.New("neither 'master' nor 'main' branch exists")
}

// commitTime reads the commit date of a revision. A worktree with no commits
// yet (or an unreadable one) simply has no date to show.
func commitTime(root, rev string) time.Time {
	if rev == "" {
		return time.Time{}
	}
	out, err := gitOut(root, "show", "-s", "--format=%ct", rev)
	if err != nil {
		return time.Time{}
	}
	ts, err := strconv.ParseInt(out, 10, 64)
	if err != nil || ts <= 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

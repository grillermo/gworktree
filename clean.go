package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/grillermo/chicle"
)

// cleanCmd backs `gworktree clean`. The list shows every worktree with the
// merged ones pre-ticked -- that is the set the non-interactive sweep would
// remove -- and lets the user tick an unmerged one too, which is then
// force-removed.
func cleanCmd(root string) error {
	worktrees, err := loadWorktrees(root)
	if err != nil {
		return err
	}
	if len(worktrees) == 0 {
		fmt.Fprintf(os.Stderr, "No worktrees under %s.\n", worktreesDir(root))
		return nil
	}

	// Get onto the base branch first either way: it is what "merged" is
	// measured against, and what the worktrees are removed from.
	base, err := ensureBase(root)
	if err != nil {
		return err
	}

	if !interactive() {
		return cleanSweep(root, base, worktrees)
	}
	return cleanPick(root, base, worktrees)
}

// cleanSweep removes every worktree whose branch is merged, without asking.
// This is what `clean` did before the picker existed, and is still what happens
// when there is no terminal to draw on.
func cleanSweep(root, base string, worktrees []worktree) error {
	failed := 0
	for _, w := range worktrees {
		if err := removeOne(root, base, w.name, false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed++
		}
	}
	return someFailed(failed)
}

// cleanPick puts the confirmation list up and carries out what comes back.
func cleanPick(root, base string, worktrees []worktree) error {
	c := newCleaner(worktrees)

	updates := make(chan []chicle.Row, 2)
	go func() {
		defer close(updates)
		for u := range liveStates(root, worktrees) {
			c.record(u.worktrees)
			updates <- rows(u.worktrees, u.column == colMerged)
		}
	}()

	chosen, err := chicle.Run(chicle.Config{
		Title:       "Clean up worktrees — enter to tick, tab for actions, merged ones are pre-ticked",
		Columns:     columns(),
		Rows:        rows(worktrees, false), // nothing is known to be merged yet
		MultiSelect: true,
		Actions:     c.actions(),
		Updates:     updates,
	})
	if err != nil || chosen == "" {
		return err
	}

	// One "<verb>\t<name>" line per tick, so each removal takes exactly the
	// same path as `gworktree remove [--force] <name>`.
	failed := 0
	for _, line := range strings.Split(chosen, "\n") {
		verb, name, _ := strings.Cut(line, "\t")
		var err error
		switch verb {
		case "remove":
			err = removeOne(root, base, name, false)
		case "force-remove":
			err = removeOne(root, base, name, true)
		default:
			err = fmt.Errorf("unexpected picker output %q", line)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed++
		}
	}
	return someFailed(failed)
}

// someFailed turns a count of failed removals into the command's exit status.
// Each failure has already explained itself on stderr.
func someFailed(n int) error {
	if n == 0 {
		return nil
	}
	return fmt.Errorf("%d worktree(s) left in place", n)
}

// cleaner holds the merged column because the action needs it: the ticked rows
// carry their state only as display text, and the verb printed for each row is
// what decides between a plain remove and a force-remove. The map is written by
// the background goroutine and read by Bubble Tea's event loop, hence the mutex.
type cleaner struct {
	mu     sync.Mutex
	merged map[string]state
}

func newCleaner(worktrees []worktree) *cleaner {
	c := &cleaner{merged: make(map[string]state, len(worktrees))}
	c.record(worktrees)
	return c
}

func (c *cleaner) record(worktrees []worktree) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range worktrees {
		c.merged[w.name] = w.merged
	}
}

// verb is what should happen to a row. Anything not known to be merged --
// including a branch whose merge check failed -- needs --force, which is also
// the honest answer: a plain remove would refuse it.
func (c *cleaner) verb(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.merged[name] == stateYes {
		return "remove"
	}
	return "force-remove"
}

func (c *cleaner) actions() []chicle.Action {
	return []chicle.Action{
		{
			Label: "Remove ticked",
			Confirm: func(s chicle.Selection) string {
				if len(s.Ticked) == 0 {
					return "" // nothing to confirm; Run says so instead
				}
				return c.question(s.Ticked)
			},
			Run: func(s chicle.Selection) chicle.Outcome {
				if len(s.Ticked) == 0 {
					return chicle.Outcome{Status: "Nothing ticked."}
				}
				return chicle.Outcome{Result: c.plan(s.Ticked), Done: true}
			},
		},
		{Label: "Exit"},
	}
}

// question spells out how much of the tick set is a force-remove, because that
// is the part that can throw away uncommitted work.
func (c *cleaner) question(ticked []chicle.Row) string {
	forced := 0
	for _, r := range ticked {
		if c.verb(r.Key) == "force-remove" {
			forced++
		}
	}
	if forced == 0 {
		return fmt.Sprintf("Remove %d merged worktree(s)?", len(ticked))
	}
	return fmt.Sprintf("Remove %d worktree(s)? %d are not merged and will be force-removed, discarding any uncommitted work.",
		len(ticked), forced)
}

// plan is the list's answer: one "<verb>\t<name>" line per ticked worktree, in
// the order they appear in the list.
func (c *cleaner) plan(ticked []chicle.Row) string {
	lines := make([]string, 0, len(ticked))
	for _, r := range ticked {
		lines = append(lines, c.verb(r.Key)+"\t"+r.Key)
	}
	return strings.Join(lines, "\n")
}

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/grillermo/chicle"
)

const (
	nameWidth   = 40
	commitWidth = 16 // "2006-01-02 15:04"
	stateWidth  = 8  // "yes" / "no" / "?" / "..." under a header
)

// columns is the table both lists draw. The last column takes whatever width is
// left, so ON REMOTE goes last.
func columns() []chicle.Column {
	return []chicle.Column{
		{Title: "WORKTREE", Width: nameWidth},
		{Title: "LAST COMMIT", Width: commitWidth},
		{Title: "MERGED", Width: stateWidth},
		{Title: "ON REMOTE"},
	}
}

// browse is the list behind a bare `gworktree`: one worktree, one action.
//
// filter, when given, pre-fills the search box. `gworktree <fragment>` uses it
// to ask "did you mean one of these?" -- the fragment is already typed, so
// Enter opens the match and Create branch is one key away if it was not.
func browse(root, filter string) error {
	worktrees, err := loadWorktrees(root)
	if err != nil {
		return err
	}
	if len(worktrees) == 0 {
		fmt.Fprintf(os.Stderr, "No worktrees under %s.\n", worktreesDir(root))
		return nil
	}

	// Plain listing when there is no terminal to draw on.
	if !interactive() {
		return listNames(root)
	}

	updates := make(chan []chicle.Row, 2)
	go func() {
		defer close(updates)
		for u := range liveStates(root, worktrees) {
			updates <- rows(u.worktrees, false)
		}
	}()

	chosen, err := chicle.Run(chicle.Config{
		Columns: columns(),
		Rows:    rows(worktrees, false),
		Filter:  filter,
		Actions: browseActions(),
		Updates: updates,
	})
	if err != nil || chosen == "" {
		return err
	}

	verb, name, _ := strings.Cut(chosen, "\t")
	switch verb {
	case "enter":
		return enter(worktreePath(root, name))
	case "remove":
		base, err := ensureBase(root)
		if err != nil {
			return err
		}
		return removeOne(root, base, name, true)
	case "create":
		return create(root, name)
	}
	return fmt.Errorf("unexpected picker output %q", chosen)
}

// createName is the branch a Create would make from the query. chicle ANDs
// whitespace-separated terms, so a query with a space is a two-term search
// rather than a name; there is no honest way to turn that into one branch
// name, so the action is simply not offered for it.
func createName(filter string) (string, bool) {
	fields := strings.Fields(filter)
	if len(fields) != 1 {
		return "", false
	}
	return fields[0], true
}

// browseActions are the buttons under the browse list. Each one only names what
// the user picked; the git work happens back in browse, so a removal chosen
// here takes exactly the same path as `gworktree remove --force <name>`.
func browseActions() []chicle.Action {
	return []chicle.Action{
		{
			Label: "Enter",
			Run: func(s chicle.Selection) chicle.Outcome {
				return chicle.Outcome{Result: "enter\t" + s.Cursor.Key, Done: true}
			},
		},
		{
			Label: "Remove",
			// The key is not truncated here: every line chicle draws, including
			// this confirm question, is passed through its own clamp, which
			// measures display width and truncates rather than wraps. A long or
			// slash-heavy worktree name can end up cut off at the terminal edge,
			// same as any other long line, but it can never wrap ugly.
			Confirm: func(s chicle.Selection) string {
				return fmt.Sprintf("Force-remove %q and its branch?", s.Cursor.Key)
			},
			Run: func(s chicle.Selection) chicle.Outcome {
				return chicle.Outcome{Result: "remove\t" + s.Cursor.Key, Done: true}
			},
		},
		{
			// The escape hatch from a search: none of these are what I meant,
			// make me the thing I typed. It is offered only once there is a
			// query, and runs even when that query matches nothing -- which is
			// the case it exists for.
			Label:   "Create branch",
			Key:     "ctrl+n",
			OnEmpty: true,
			Show: func(s chicle.Selection) bool {
				_, ok := createName(s.Filter)
				return ok
			},
			// The name comes from a search box, which is an unusual place for a
			// branch name to come from, so it is shown back before anything is
			// created.
			Confirm: func(s chicle.Selection) string {
				name, _ := createName(s.Filter)
				return fmt.Sprintf("Create worktree and branch %q?", name)
			},
			Run: func(s chicle.Selection) chicle.Outcome {
				name, ok := createName(s.Filter)
				if !ok {
					return chicle.Outcome{Status: "Narrow to a single term to use it as a branch name."}
				}
				return chicle.Outcome{Result: "create\t" + name, Done: true}
			},
		},
		{Label: "Exit"},
	}
}

// rows renders the table. tick pre-ticks the already-merged rows, which only
// the clean list wants, and only on the update that first knows which they are.
// chicle ORs an incoming tick with what is already ticked, so passing false
// later leaves the user's own ticks -- and unticks -- alone.
func rows(worktrees []worktree, tick bool) []chicle.Row {
	out := make([]chicle.Row, 0, len(worktrees))
	for _, w := range worktrees {
		out = append(out, chicle.Row{
			Key:    w.name,
			Cols:   []string{w.name, w.lastCommitLabel(), w.merged.label(), w.onRemote.label()},
			Ticked: tick && w.merged == stateYes,
		})
	}
	return out
}

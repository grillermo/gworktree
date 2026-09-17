package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The integration is a file rather than a Go string literal so it stays
// syntax-highlighted, shellcheck-able and editable as zsh.
//
//go:embed shell/gworktree.zsh
var zshInit string

// shellInit prints the shell integration for
//
//	eval "$(~/c/gworktree/bin/gworktree --shell-init zsh)"
//
// That one line is the whole install: it puts this binary on PATH and defines
// the wrapper that turns its answer into a cd. Only zsh is supported, because
// the completion function is zsh's.
func shellInit(args []string) error {
	shell := "zsh"
	if len(args) > 0 {
		shell = args[0]
	}
	if shell != "zsh" {
		return fmt.Errorf("no shell integration for %q; only zsh", shell)
	}

	// The anonymous function is only there to keep $dir out of the caller's
	// scope; the (Ie) subscript needs a variable, since a quoted literal in
	// there is matched with its quotes on.
	if dir, err := binDir(); err == nil {
		fmt.Printf(`# gworktree's own directory, so `+"`command gworktree`"+` resolves.
() {
  local dir=%s
  (( ${path[(Ie)$dir]} )) || path=($dir $path)
}

`, zshQuote(dir))
	}
	fmt.Print(zshInit)
	return nil
}

// binDir is where this binary lives, resolved through any symlink so that a
// linked-to-somewhere-else gworktree still puts its real directory on PATH.
func binDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolve(exe)), nil
}

// zshQuote wraps a string in single quotes, the one zsh quoting that leaves
// everything inside alone.
func zshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

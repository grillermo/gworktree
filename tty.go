package main

import "os"

// interactive reports whether there is a terminal to draw the picker on.
//
// It asks /dev/tty rather than isatty(stdout): under the shell wrapper stdout
// is the terminal, but under `gworktree > file` it is not, and either way the
// picker draws to /dev/tty. What matters is whether a controlling terminal
// exists at all -- in a cron job or a `ssh host gworktree`, it does not.
func interactive() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	tty.Close()
	return true
}

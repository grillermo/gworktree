# zsh integration for gworktree, printed by `gworktree --shell-init zsh`.
#
# The binary does all the git work. The one thing it cannot do is change this
# shell's directory, so it writes the destination to a scratch file and the
# wrapper below does the cd.

gworktree() {
  local cdfile
  cdfile=$(mktemp "${TMPDIR:-/tmp}/gworktree.XXXXXX") || return 1

  GWORKTREE_CD_FILE=$cdfile command gworktree "$@"
  local ret=$?

  local dest
  dest=$(<"$cdfile")
  rm -f -- "$cdfile"

  # A destination is written whether or not the command as a whole succeeded
  # (removing a worktree you are standing in steps you out of it first), so the
  # cd is not conditional on $ret -- but its own failure is not allowed to
  # rewrite the command's status either.
  [[ -n $dest ]] && cd -- "$dest"

  return $ret
}

_gworktree() {
  local -a worktrees
  worktrees=(${(f)"$(command gworktree --list 2>/dev/null)"})

  case $CURRENT in
    2)
      _alternative \
        'subcommands:subcommand:(remove clean cleanup)' \
        "worktrees:worktree:($worktrees)"
      ;;
    *)
      # `remove` takes any number of worktrees; offer the ones not yet named
      case $words[2] in
        remove)
          compadd -F words -a worktrees
          compadd -F words -- --force
          ;;
      esac
      ;;
  esac
}
# compdef only exists once compinit has run. Somewhere that never happens --
# a `zsh -f`, a script -- the wrapper is still worth having without it.
(( $+functions[compdef] )) && compdef _gworktree gworktree

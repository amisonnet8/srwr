# fish completion for srwr.
#
# It has no list of commands. At every TAB it asks `srwr __complete` what can
# come next, so it is never out of date with the srwr that is installed, and
# it can offer the tape IDs of the workspace.
#
# Install it with:  srwr completion fish > ~/.config/fish/completions/srwr.fish
# or try it with:   srwr completion fish | source

function __srwr_complete
    # The words before the one being typed, without "srwr" and their quotes.
    set -l words (commandline -opc)
    set -e words[1]
    # The word being typed, without its quotes and backslashes, as --word: an
    # empty word would be dropped, and "--word=" never is.
    set -l cur (commandline -ct | string replace -ra "[\\\\'\"]" '')
    srwr __complete "--word=$cur" -- $words 2>/dev/null
end

# Where a file is wanted: after --root, and the file trace reads.
function __srwr_wants_file
    set -l words (commandline -opc)
    set -e words[1]
    test (count $words) -ge 1; and test "$words[-1]" = --root; and return 0
    test (count $words) -ge 1; and test "$words[1]" = trace
end

complete -c srwr -f -a '(__srwr_complete)'
complete -c srwr -F -n __srwr_wants_file

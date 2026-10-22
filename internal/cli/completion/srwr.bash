# bash completion for srwr.
#
# It has no list of commands. At every TAB it asks `srwr __complete` what can
# come next, so it is never out of date with the srwr that is installed, and
# it can offer the tape IDs of the workspace.
#
# Install it with:  srwr completion bash > ~/.local/share/bash-completion/completions/srwr
# or try it with:   source <(srwr completion bash)

_srwr() {
    local value
    local -a words=("${COMP_WORDS[@]:1:COMP_CWORD-1}")
    COMPREPLY=()
    # The word being typed goes as --word: an empty word would be dropped by
    # some shells, and "--word=" never is. bash splits --root=x at "=", and
    # srwr __complete puts such words back together.
    while IFS=$'\t' read -r value _; do
        COMPREPLY+=("$value")
    done < <(command srwr __complete "--word=${COMP_WORDS[COMP_CWORD]}" -- "${words[@]}" 2>/dev/null)
}

# -o default: where nothing is offered (--root, the file of trace), files are.
complete -o default -F _srwr srwr

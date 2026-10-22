#compdef srwr
# zsh completion for srwr.
#
# It has no list of commands. At every TAB it asks `srwr __complete` what can
# come next, so it is never out of date with the srwr that is installed, and
# it can offer the tape IDs of the workspace.
#
# Install it with:  srwr completion zsh > "${fpath[1]}/_srwr"
# or try it with:   source <(srwr completion zsh)
_srwr() {
    local line value item
    local -a items
    # The words before the one being typed, and that one as --word, without
    # their quotes: an empty word would be dropped by some shells, and
    # "--word=" never is.
    for line in "${(@f)$(command srwr __complete "--word=${(Q)words[CURRENT]}" -- "${(@Q)words[2,CURRENT-1]}" 2>/dev/null)}"; do
        [[ -n $line ]] || continue
        # "value<TAB>desc" is "value:desc" for _describe, which wants a colon
        # in the value written as \:.
        value=${line%%$'\t'*}
        item=${value//:/\\:}
        [[ $line == *$'\t'* ]] && item+=":${line#*$'\t'}"
        items+=("$item")
    done
    if (( ${#items} == 0 )); then
        # Where nothing is offered (--root, the file of trace), files are.
        _files
        return
    fi
    _describe -t candidates 'srwr' items
    return 0
}

if [ "$funcstack[1]" = "_srwr" ]; then
    _srwr "$@"
else
    compdef _srwr srwr
fi

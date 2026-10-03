" srwr-view.vim: replays what an AI did with srwr, frame by frame (docs/reference/vim.md).
" Only an if ... finish block may come before vim9script.
if !has('vim9script') || !has('patch-9.0.0784') || !has('channel') || !has('job') || !has('textprop')
  echomsg 'srwr-view.vim: this Vim cannot run it. Needs Vim 9.0.0784 or later with +channel +job +textprop +vim9script. Missing: '
        \ .. join(filter(['vim9script', 'channel', 'job', 'textprop'], '!has(v:val)') + (has('patch-9.0.0784') ? [] : ['patch 9.0.0784']), ', ')
  finish
endif

vim9script

if exists('g:loaded_srwr')
  finish
endif
g:loaded_srwr = 1

import autoload 'srwr/ui.vim'

command! -nargs=? SrwrOpen ui.Open(<q-args>)
command! SrwrLive ui.Live()
command! SrwrNext ui.Next()
command! SrwrPrev ui.Prev()
command! SrwrClose ui.Close()
command! SrwrLatest ui.Latest()

" srwr-view.vim: replays what an AI did with srwr, frame by frame (docs/reference/vim.md).
" Only an if ... finish block may come before vim9script.
" No line continuation here: 'compatible' (vim -u NONE) turns it off.
if !has('vim9script') || !has('patch-9.0.0784') || !has('channel') || !has('job') || !has('textprop')
  echomsg 'srwr-view.vim: this Vim cannot run it. Needs Vim 9.0.0784 or later with +channel +job +textprop +vim9script. Missing: ' . join(filter(['vim9script', 'channel', 'job', 'textprop'], '!has(v:val)') + (has('patch-9.0.0784') ? [] : ['patch 9.0.0784']), ', ')
  finish
endif

vim9script noclear

if exists('g:loaded_srwr')
  finish
endif
g:loaded_srwr = 1

import autoload 'srwr/ui.vim'

# A user command is not run in this script's context, so it cannot see the import: it calls these functions instead.
def Open(arg: string)
  ui.Open(arg)
enddef

def Live()
  ui.Live()
enddef

def Next()
  ui.Next()
enddef

def Prev()
  ui.Prev()
enddef

def Close()
  ui.Close()
enddef

def Latest()
  ui.Latest()
enddef

def Toggle(kind: string)
  ui.Toggle(trim(kind))
enddef

command! -nargs=? SrwrOpen call <SID>Open(<q-args>)
command! SrwrLive call <SID>Live()
command! SrwrNext call <SID>Next()
command! SrwrPrev call <SID>Prev()
command! SrwrClose call <SID>Close()
command! SrwrLatest call <SID>Latest()
command! -nargs=1 SrwrToggle call <SID>Toggle(<q-args>)

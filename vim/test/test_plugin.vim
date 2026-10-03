vim9script
# The commands the plugin defines, as a person types them.
import './helpers.vim' as t
t.Setup()

for name in ['SrwrOpen', 'SrwrLive', 'SrwrNext', 'SrwrPrev', 'SrwrClose', 'SrwrLatest']
  t.Equal(0, exists(':' .. name), name .. ' does not exist before the plugin is loaded')
endfor
runtime plugin/srwr.vim
for name in ['SrwrOpen', 'SrwrLive', 'SrwrNext', 'SrwrPrev', 'SrwrClose', 'SrwrLatest']
  t.Equal(2, exists(':' .. name), name .. ' exists')
endfor
t.Equal(1, g:loaded_srwr, 'loaded once')

# Loading it again changes nothing, and with nothing open the commands do nothing.
runtime plugin/srwr.vim
SrwrNext
SrwrPrev
SrwrLatest
SrwrClose

import autoload 'srwr/replay.vim'
set columns=140 lines=50
t.Workspace()
SrwrOpen 20260930-0054-why-basic
t.WaitFor((): bool => replay.Active(), 'the tape to open')
SrwrNext
t.Equal(1, replay.Session().index, ':SrwrNext')
SrwrPrev
t.Equal(0, replay.Session().index, ':SrwrPrev')
SrwrClose
t.Equal(false, replay.Active(), ':SrwrClose')
t.Equal(1, tabpagenr('$'), 'no tab is left')

t.Finish()

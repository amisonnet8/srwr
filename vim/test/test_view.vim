vim9script
# The replay screen against the real display server (bin/srwr, built by `qsoku bin`).
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/replay.vim'
import autoload 'srwr/sidebar.vim'
import autoload 'srwr/ui.vim'

const WHY = '20260930-0054-why-basic'
const NOWHY = '20260930-0053-no-why'

def Types(b: number, lnum: number): list<string>
  return uniq(sort(mapnew(prop_list(lnum, {bufnr: b}), (_, p) => p.type)))
enddef

def Srwr(): list<string>
  return filter(mapnew(getbufinfo(), (_, b) => b.name), (_, n) => n =~# '^srwr://')
enddef

def OpenTape(id: string)
  ui.Open(id)
  t.WaitFor((): bool => replay.Active(), 'the tape ' .. id .. ' to open')
enddef

# A window as wide as a terminal people use, so that the why fits in one row as in the images.
set columns=140 lines=50
t.Workspace()
const tabsBefore = tabpagenr('$')

# --- frame 0: a select with a why ---
OpenTape(WHY)
var s = replay.Session()
t.Equal(tabsBefore + 1, tabpagenr('$'), 'one tab is added')
t.Equal(2, winnr('$'), 'the list and the frame')
# The status line of the list is its name only: Vim's own would add the cursor position and a word like "All" in the language of the Vim.
t.Equal('%f', getwinvar(sidebar.Win(), '&statusline'), 'the list has a status line of its own')
t.Equal('srwr://' .. WHY .. '/text.go', bufname(s.buf), 'buffer name')
t.Equal(7, line('$', s.sidebar), 'seven rows in the list')
t.Equal('●  1 select  text.go:37  Truncate が幅ちょうどの文字列まで切り詰めてしまう原因の比較。', getbufline(winbufnr(s.sidebar), 1)[0], 'first row of the list: the dot, then the number (the order of the VSCode list)')
t.Equal([{col: 1, type: 'srwr_dot_select'}], mapnew(filter(prop_list(1, {bufnr: winbufnr(s.sidebar)}), (_, p) => p.type =~# '^srwr_dot'), (_, p) => ({col: p.col, type: p.type})), 'the dot is colored')
const lines = getbufline(s.buf, 1, '$')
t.True(lines[36] =~# '^◆ Truncate が幅ちょうど', 'the why row sits in front of the range')
t.True(lines[37] =~# '^\tif DisplayWidth(s) < w {', 'the range follows')
t.Equal(['srwr_why_select'], Types(s.buf, 37)->filter((_, v) => v !=# 'srwr_num'), 'why row color')
t.Equal(['srwr_select'], Types(s.buf, 38)->filter((_, v) => v !=# 'srwr_num'), 'range color')
t.Equal([], Types(s.buf, 36)->filter((_, v) => v !=# 'srwr_num'), 'other rows are not painted')
t.Equal(len(lines), len(filter(mapnew(range(1, len(lines)), (_, l) => Types(s.buf, l)), (_, v) => index(v, 'srwr_num') >= 0)), 'a number on every row')
t.Equal(false, getwinvar(s.win, '&number'), 'standard numbers are off while there are why rows')
# prop_list() does not return the text of virtual text; the labels are tested in test_pure.vim and on the screen (test_screen).
t.True(getwinvar(s.win, '&statusline') =~# 'srwr  1/7  %#SrwrDim#\[\[ 戻る%\*  \]\] 進む%<  text.go:37', 'status line at the first frame')
const info = getwininfo(s.win)[0]
t.True(info.topline <= 37 && 38 <= info.topline + info.height - 1, 'the why row and the range are on the screen')

# --- steps ---
replay.StepBack()
t.Equal(0, s.index, 'back at the first frame stays')
for _ in range(3)
  replay.StepForward()
endfor
t.Equal(3, s.index, 'three steps forward')
t.Equal('srwr://' .. WHY .. '/parse.go', bufname(s.buf), 'the buffer follows the file')
t.Equal(2, winnr('$'), 'still two windows')
replay.Jump(4)
t.Equal(['srwr_why_replace'], Types(s.buf, 37)->filter((_, v) => v !=# 'srwr_num'), 'replace why row is orange')
t.True(getbufline(s.buf, 1, '$')->join("\n") =~# 'DisplayWidth(s) <= w', 'the content after the replace')
t.Equal(['srwr_replace'], Types(s.buf, 38)->filter((_, v) => v !=# 'srwr_num'), 'replace range is orange')
for _ in range(20)
  replay.StepForward()
endfor
t.Equal(6, s.index, 'forward stops at the last frame')
t.True(getwinvar(s.win, '&statusline') =~# '%#SrwrDim#\]\] 進む', 'forward is dim at the last frame')
t.Equal(sort([bufname(s.buf), 'srwr://operations']), sort(Srwr()), 'only two srwr buffers after all those steps')
t.Equal(7, line('.', s.sidebar), 'the cursor of the list is on the current frame')

# --- closing ---
ui.Close()
t.Equal(tabsBefore, tabpagenr('$'), 'the tab is gone')
t.Equal([], Srwr(), 'no buffer is left')
t.Equal(false, replay.Active(), 'not active')

# --- a tape without why: standard numbers, no why row ---
OpenTape(NOWHY)
s = replay.Session()
t.Equal(true, getwinvar(s.win, '&number'), 'standard numbers on')
t.Equal([], filter(range(1, line('$', s.win)), (_, l) => index(Types(s.buf, l), 'srwr_num') >= 0), 'no own numbers')
t.True(getbufline(s.buf, 1, 1)[0] !~# '◆', 'no why row')
t.Equal(['srwr_select'], Types(s.buf, 1), 'the range is painted from line 1')
ui.Close()

t.Finish()

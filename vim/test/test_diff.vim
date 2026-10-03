vim9script
# Diff frames (external, final): two windows, changed lines only, and nothing piles up.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/replay.vim'
import autoload 'srwr/ui.vim'

const EXT = '20260930-0949-external'

def Types(b: number, lnum: number): list<string>
  return uniq(sort(mapnew(prop_list(lnum, {bufnr: b}), (_, p) => p.type)))
enddef

def Painted(b: number, type: string): list<number>
  return filter(range(1, getbufinfo(b)[0].linecount), (_, l) => index(Types(b, l), type) >= 0)
enddef

def Srwr(): list<string>
  return sort(filter(mapnew(getbufinfo(), (_, b) => b.name), (_, n) => n =~# '^srwr://'))
enddef

set columns=140 lines=50
t.Workspace()
const diffColorsBefore = mapnew(['DiffAdd', 'DiffChange', 'DiffDelete', 'DiffText'], (_, g) => hlget(g))

ui.Open(EXT)
t.WaitFor((): bool => replay.Active(), 'the tape to open')
const s = replay.Session()

# --- an external frame (frame 5) ---
replay.Jump(5)
t.Equal(3, winnr('$'), 'the list, the left and the right window')
t.True(s.diffWin > 0 && win_id2win(s.diffWin) > 0, 'the right window exists')
t.Equal('srwr://' .. EXT .. '/before/stats.go', bufname(s.buf), 'left buffer')
t.Equal('srwr://' .. EXT .. '/after/stats.go', bufname(s.diffBuf), 'right buffer')
t.Equal(true, getwinvar(s.win, '&diff') && getwinvar(s.diffWin, '&diff'), 'both are in diff mode')
t.Equal(false, getbufvar(s.buf, '&modifiable'), 'left is read-only')
t.Equal(false, getbufvar(s.diffBuf, '&modifiable'), 'right is read-only')
t.Equal([24, 30, 33], Painted(s.buf, 'srwr_select'), 'changed lines on the left, blue')
t.Equal([24, 30, 33], Painted(s.diffBuf, 'srwr_replace'), 'changed lines on the right, orange')
t.Equal([], Painted(s.buf, 'srwr_replace'), 'no orange on the left')
t.Equal(true, getwinvar(s.win, '&number'), 'standard numbers on the left')
t.Equal([], Painted(s.buf, 'srwr_num'), 'no own numbers in a diff')
t.Equal('前  ⚠ srwrの外で変更：stats.go', getwinvar(s.win, '&statusline'), 'left heading')
t.True(getwinvar(s.diffWin, '&statusline') =~# '^後  srwr  6/11  ', 'right status line')
# Unchanged lines are folded in diff mode, so where the window starts is tested on a terminal (the screen test).
t.True(getwininfo(s.win)[0].topline <= 24, 'the first changed line is on the screen')
t.Equal(24, line('.', s.win), 'and the cursor is on it')
# Vim's own diff colors are taken away while the frame is shown.
for g in ['DiffAdd', 'DiffChange', 'DiffDelete', 'DiffText']
  t.Equal(true, get(hlget(g)[0], 'cleared', false) || len(hlget(g)[0]) <= 2, g .. ' is taken away')
endfor

# --- going back to a normal frame ---
replay.Jump(6)
t.Equal(2, winnr('$'), 'back to the list and one window')
t.Equal(false, getwinvar(s.win, '&diff'), 'diff mode is off')
t.Equal(0, s.diffWin, 'no right window recorded')
t.Equal(diffColorsBefore, mapnew(['DiffAdd', 'DiffChange', 'DiffDelete', 'DiffText'], (_, g) => hlget(g)), "Vim's colors are back")
t.Equal(2, len(Srwr()), 'only the list and one frame buffer are left')

# --- the final frames (9 and 10) ---
replay.Jump(9)
t.Equal('前  ⚠ 録画のあとで変更（今のファイルとの差分）：entry.go', getwinvar(s.win, '&statusline'), 'final heading')
t.Equal('srwr://' .. EXT .. '/after/entry.go', bufname(s.diffBuf), 'right buffer follows the file')
replay.Jump(10)
t.Equal('前  ⚠ 録画のあとで変更（今のファイルとの差分）：stats.go', getwinvar(s.win, '&statusline'), 'the next final frame')
t.Equal(3, winnr('$'), 'still three windows from diff to diff')

# --- back and forth does not pile up ---
for _ in range(8)
  replay.Jump(5)
  replay.Jump(7)
  replay.Jump(9)
  replay.Jump(2)
endfor
t.Equal(2, winnr('$'), 'two windows after going back and forth')
t.Equal(2, len(Srwr()), 'the same number of buffers as at the start')
t.Equal(1, tabpagenr('$') - 1, 'one tab for the replay')

ui.Close()
t.Equal(1, tabpagenr('$'), 'the tab is gone')
t.Equal([], Srwr(), 'no srwr buffer is left')
t.Equal(diffColorsBefore, mapnew(['DiffAdd', 'DiffChange', 'DiffDelete', 'DiffText'], (_, g) => hlget(g)), "Vim's colors are back after closing")

t.Finish()

vim9script
# The keys work in srwr's own buffers and nowhere else.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/list.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/ui.vim'

set columns=140 lines=50
t.Workspace()
t.Equal('', maparg(']]', 'n'), 'no global ]]')
t.Equal('', maparg('q', 'n'), 'no global q')
t.Equal('', maparg('L', 'n'), 'no global L')

ui.Open('20260930-0949-external')
t.WaitFor((): bool => replay.Active(), 'the tape to open')
const s = replay.Session()
for w in [s.win, s.sidebar]
  for key in [']]', '[[', '<Right>', '<Left>', 'q']
    t.Equal(1, win_execute(w, 'echo maparg(' .. string(key) .. ', "n", 0, 1).buffer')->trim()->str2nr(), key .. ' is local in window ' .. w)
  endfor
endfor
t.Equal(1, win_execute(s.sidebar, 'echo maparg("<CR>", "n", 0, 1).buffer')->trim()->str2nr(), '<CR> is local in the list')
t.Equal(0, win_execute(s.win, 'echo maparg("L", "n", 0, 1)->get("buffer", 0)')->trim()->str2nr(), 'L is for live only')

# the keys step through the frames
win_gotoid(s.win)
execute 'normal ]]'
t.Equal(1, s.index, ']] steps forward')
execute "normal \<Right>"
t.Equal(2, s.index, '<Right> steps forward')
execute 'normal [['
t.Equal(1, s.index, '[[ steps back')
execute "normal \<Left>"
t.Equal(0, s.index, '<Left> steps back')
# the list: <CR> goes to the row
win_gotoid(s.sidebar)
cursor(6, 1)
execute "normal \<CR>"
t.Equal(5, s.index, '<CR> goes to the frame of the row')
t.Equal(3, winnr('$'), 'it is a diff frame: three windows')
# the right-hand window of a diff has the keys too
t.Equal(1, win_execute(s.diffWin, 'echo maparg("]]", "n", 0, 1).buffer')->trim()->str2nr(), 'the keys work in the diff window')
win_gotoid(s.diffWin)
execute 'normal [['
t.Equal(4, s.index, '[[ in the right-hand window')
win_gotoid(s.win)

# many presses in a row: the position follows every press and stops at both ends
win_gotoid(s.win)
replay.Jump(0)
const last = len(s.tl.frames) - 1
for _ in range(last + 15)
  execute 'normal ]]'
endfor
t.Equal(last, s.index, 'many ]] stop at the last frame')
for _ in range(last + 15)
  execute 'normal [['
endfor
t.Equal(0, s.index, 'many [[ stop at the first frame')
t.Equal(2, winnr('$'), 'no window piled up while pressing')
win_gotoid(s.win)

# q closes
execute 'normal q'
t.Equal(false, replay.Active(), 'q closes the replay')
t.Equal(1, tabpagenr('$'), 'and its tab')
t.Equal('', maparg(']]', 'n'), 'nothing leaks out of the buffers')

t.Finish()

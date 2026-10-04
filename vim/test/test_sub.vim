vim9script
# A sub frame (a replace of several places of a file): two windows like a diff, and the why in a band above both.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/replay.vim'
import autoload 'srwr/ui.vim'
import autoload 'srwr/sidebar.vim'

def Types(b: number, lnum: number): list<string>
  return uniq(sort(mapnew(prop_list(lnum, {bufnr: b}), (_, p) => p.type)))
enddef

def Painted(b: number, type: string): list<number>
  return filter(range(1, getbufinfo(b)[0].linecount), (_, l) => index(Types(b, l), type) >= 0)
enddef

set columns=140 lines=50
const dir = tempname()
mkdir(dir, 'p')
execute 'cd ' .. fnameescape(dir)
writefile(['func a() {', '  foo(1)', '  x', '  foo(2)', '}'], 'a.go')
writefile(['foo(3)'], 'b.go')

# A real tape: srwr mcp makes it, with the sub tool.
const calls = [
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}',
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"sub","arguments":{"files":["a.go","b.go"],"old":"foo(","new":"bar(","count":3,"why":"名前を変える"}}}',
]
const answer = system(shellescape(g:srwr_path) .. ' mcp', join(calls, "\n") .. "\n")
t.True(answer =~# 'hits\\":2', 'sub answered: ' .. answer)
t.Equal(['func a() {', '  bar(1)', '  x', '  bar(2)', '}'], readfile('a.go'), 'a.go was changed')
const tapeId = trim(join(readfile('.srwr/active'), ''))

ui.Open(tapeId)
t.WaitFor((): bool => replay.Active(), 'the tape to open')
const s = replay.Session()

# One frame for each file that changed.
t.Equal(2, len(s.tl.frames), 'two frames')
t.Equal('sub', s.tl.frames[0].kind, 'the kind')
t.Equal(2, s.tl.frames[0].hits, 'hits')

replay.Jump(0)
t.Equal(3, winnr('$'), 'the list, the left and the right window')
t.Equal('srwr://' .. tapeId .. '/before/a.go', bufname(s.buf), 'left buffer')
# The band: one row of the why on the right, an empty row on the left, so the lines line up.
t.Equal('', getbufline(s.buf, 1)[0], 'empty row on the left')
t.Equal('◆ 名前を変える', trim(getbufline(s.diffBuf, 1)[0]), 'the why on the right')
t.True(getbufline(s.diffBuf, 1)[0] =~# '^  ', 'and it is centered')
t.Equal(['', 'func a() {', '  foo(1)', '  x', '  foo(2)', '}'], getbufline(s.buf, 1, '$'), 'before, below the band')
t.Equal(['func a() {', '  bar(1)', '  x', '  bar(2)', '}'], getbufline(s.diffBuf, 2, '$'), 'after, below the band')
t.Equal([1], Painted(s.buf, 'srwr_why_select'), 'the band on the left is blue')
t.Equal([1], Painted(s.diffBuf, 'srwr_why_replace'), 'the band on the right is orange')
t.Equal([3, 5], Painted(s.buf, 'srwr_select'), 'changed lines on the left, blue')
t.Equal([3, 5], Painted(s.diffBuf, 'srwr_replace'), 'changed lines on the right, orange')
t.Equal(false, getwinvar(s.win, '&number'), 'standard numbers are off: the band would make them wrong')
t.Equal(range(1, 6), Painted(s.diffBuf, 'srwr_num'), 'own numbers are drawn on every row of the right')
t.Equal(range(1, 6), Painted(s.buf, 'srwr_num'), 'and of the left')
t.Equal('前  ⚠ sub：a.go', getwinvar(s.win, '&statusline'), 'left heading')
t.True(getwinvar(s.diffWin, '&statusline') =~# '^後  srwr  1/2  ', 'right status line')

# The list: the file and the number of places.
t.Equal('sub', sidebar.Line(s.tl.frames[0])[5 : 7], 'kind in the list')
t.True(sidebar.Line(s.tl.frames[0]) =~# 'a\.go (2か所)', 'places in the list: ' .. sidebar.Line(s.tl.frames[0]))
t.True(sidebar.Line(s.tl.frames[1]) =~# 'b\.go (1か所)', 'one place: ' .. sidebar.Line(s.tl.frames[1]))
t.Equal('srwr_dot_replace', sidebar.DotType('sub'), 'orange dot')

# The next frame is another file; going back to a normal frame is not needed here, the windows stay.
replay.Jump(1)
t.Equal('srwr://' .. tapeId .. '/after/b.go', bufname(s.diffBuf), 'the next file')
t.Equal(3, winnr('$'), 'still three windows')

t.Finish()

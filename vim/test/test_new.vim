vim9script
# A new frame (a file made by the new tool): the whole file in one window, painted like a replace, with its why above.
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

# A real tape: srwr mcp makes it, with the new tool.
const calls = [
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}',
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"new","arguments":{"file":"a.go","content":"package a\n\nvar X = 1","why":"パッケージを作る"}}}',
]
const answer = system(shellescape(g:srwr_path) .. ' mcp', join(calls, "\n") .. "\n")
t.True(answer =~# 'startLine\\":1', 'new answered: ' .. answer)
t.Equal(['package a', '', 'var X = 1'], readfile('a.go'), 'a.go was made')
const tapeId = trim(join(readfile('.srwr/active'), ''))

ui.Open(tapeId)
t.WaitFor((): bool => replay.Active(), 'the tape to open')
const s = replay.Session()

t.Equal(1, len(s.tl.frames), 'one frame')
t.Equal('new', s.tl.frames[0].kind, 'the kind')
replay.Jump(0)
t.Equal(2, winnr('$'), 'the list and one window')
t.Equal('◆ パッケージを作る', trim(getbufline(s.buf, 1)[0]), 'the why above the file')
t.Equal(['package a', '', 'var X = 1'], getbufline(s.buf, 2, '$'), 'the content')
t.Equal([1], Painted(s.buf, 'srwr_why_replace'), 'the why is orange')
t.Equal([2, 3, 4], Painted(s.buf, 'srwr_replace'), 'the whole file is painted orange')

t.True(sidebar.Line(s.tl.frames[0]) =~# 'new     a\.go:1-3', 'the list: ' .. sidebar.Line(s.tl.frames[0]))
t.Equal('srwr_dot_replace', sidebar.DotType('new'), 'orange dot')

t.Finish()

vim9script
# Which kinds of frames are shown, and the frame of a failure.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/list.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/sidebar.vim'
import autoload 'srwr/timeline.vim'
import autoload 'srwr/ui.vim'

# --- pure ---
const fail1 = {index: 1, seq: 3, kind: 'failure', file: '', range: {start: 9, end: 9}, why: 'abs', before: '', after: '', tool: 'look', code: 'invalid_range', message: 'The path is absolute. Give a path relative to the workspace'}
const fail2 = {index: 3, seq: 5, kind: 'failure', file: 'a.go', range: {start: 0, end: -1}, why: v:null, before: '', after: '', tool: 'edit', code: 'selection_stale', message: 'stale'}
const frames = [
  {index: 0, seq: 2, kind: 'look', file: 'a.go', range: {start: 2, end: 2}, why: 'look', before: "1\n2\n3\n", after: "1\n2\n3\n"},
  fail1,
  {index: 2, seq: 4, kind: 'edit', file: 'a.go', range: {start: 2, end: 2}, why: 'change', before: "1\n2\n3\n", after: "1\nTWO\n3\n"},
  fail2,
]
const tl = timeline.New(frames)
t.Equal('failure', timeline.Tone(fail1), 'tone of a failure')
t.Equal(false, timeline.IsDiff(fail1), 'a failure is not a diff frame')
t.Equal("1\nTWO\n3\n", timeline.ContentAt(tl, 'a.go', 3), 'a failure names a file but holds no text')
t.Equal("1\n2\n3\n", timeline.ContentAt(tl, 'a.go', 1), 'the content before the replace')
t.Equal(2, timeline.NearestBySeq(tl, 4), 'nearest by seq')
t.Equal(3, timeline.NearestBySeq(tl, 100), 'nearest by seq, past the end')
t.Equal(0, timeline.NearestBySeq(tl, 1), 'nearest by seq, before the start')
t.Equal(-1, timeline.NearestBySeq(timeline.New(), 4), 'no frames')
t.Equal(0, timeline.NearestBySeq(timeline.New([{index: 0, seq: 2, kind: 'look'}, {index: 1, seq: 4, kind: 'look'}]), 3), 'a tie goes to the earlier frame')
t.Equal('', timeline.HiddenText({}), 'nothing hidden')
t.Equal('隠している: look (1), failure (2)', timeline.HiddenText({failure: 2, look: 1}), 'hidden, in the order of the kinds')
t.Equal(['✖ look が失敗しました  (invalid_range)', fail1.message, '', 'why    abs', 'tool   look', 'range  9 行', 'file   (not shown)'], timeline.FailureLines(fail1), 'a look failure')
t.Equal(['✖ edit が失敗しました  (selection_stale)', 'stale', '', 'why    (なし)', 'tool   edit', 'range  -', 'file   a.go'], timeline.FailureLines(fail2), 'an edit failure')
t.Equal('●  2 失敗     invalid_range  abs', sidebar.Line(fail1, ), 'a failure row: the code in the place of file:range')
t.Equal('srwr_dot_failure', sidebar.DotType('failure'), 'a red dot')
const plain = join(mapnew(replay.StatusParts(1, 4, false, 'failure: select', 100, '', 'hidden: failure (2)'), (_, p) => p[0]), '')
t.Equal('srwr  2/4  [[ 戻る  ]] 進む  failure: select  hidden: failure (2)', plain, 'the status line with what is left out')
t.Equal('%=', matchstr(replay.StatusString(replay.StatusParts(1, 4, false, '', 100, '', 'x')), '%='), 'the hidden text goes to the right end')

# --- a failure frame on the screen ---
set columns=140 lines=50
t.Workspace(false)
t.Equal('', maparg('tf', 'n'), 'no global tf')
replay.Open('20261004-1530-kinds', frames, getcwd(), false, {failure: 2}, 1)
const s = replay.Session()
t.Equal(1, s.index, 'it starts at the frame asked for')
t.Equal(timeline.FailureLines(fail1), getbufline(s.buf, 1, '$'), 'the failure is explained in the buffer')
t.True(!empty(prop_list(1, {bufnr: s.buf, types: ['srwr_why_failure']})), 'the first row is red')
t.True(empty(prop_list(2, {bufnr: s.buf, types: ['srwr_why_failure']})), 'only the first row')
t.Equal(false, getwinvar(s.win, '&number'), 'no line numbers')
t.Equal('', getbufvar(s.buf, '&filetype'), 'no filetype: the colors of the last file do not stay')
t.Equal('srwr://20261004-1530-kinds/failure1', bufname(s.buf), 'a name that is not a file')
const status = getwinvar(s.win, '&statusline')
t.True(status =~# 'failure: select' && status =~# 'hidden: failure (2)' || status =~# '隠している: failure (2)', 'the status line: ' .. status)
t.Equal(hlget('SrwrWhyFailure')[0].guibg, '#d50000', 'the red of the why row')
replay.Goto(2)
t.True(getbufline(s.buf, 1, '$')[0] !~# '✖', 'a replace is shown as usual after it')
t.Equal(false, getwinvar(s.win, '&number'), 'and its why row has the numbers of the file')
replay.Close()

# --- the kinds, against the real server ---
const dir = t.Workspace(false)
const tapeFile = dir .. '/.srwr/tapes/20261004-1530-kinds.tape.jsonl'
writefile([
  '{"v":1,"type":"header","session":"s","startedAt":"2026-10-04T15:30:00.000Z"}',
  '{"v":1,"seq":1,"ts":"2026-10-04T15:30:00.000Z","type":"snapshot","file":"a.go","text":"1\n2\n3\n"}',
  '{"v":1,"seq":2,"ts":"2026-10-04T15:30:01.000Z","type":"select","file":"a.go","startLine":2,"endLine":2,"why":"look","selection":"sel_A"}',
  '{"v":1,"seq":3,"ts":"2026-10-04T15:30:02.000Z","type":"failure","tool":"select","file":null,"startLine":9,"endLine":9,"selection":null,"why":"abs","code":"invalid_range","message":"The path is absolute. Give a path relative to the workspace"}',
  '{"v":1,"seq":4,"ts":"2026-10-04T15:30:03.000Z","type":"replace","file":"a.go","from":"sel_A","startLine":2,"endLine":2,"oldText":"2","newText":"TWO","newStartLine":2,"newEndLine":2,"selection":"sel_B","why":"change"}',
  '{"v":1,"seq":5,"ts":"2026-10-04T15:30:04.000Z","type":"failure","tool":"replace","file":"a.go","startLine":null,"endLine":null,"selection":"sel_A","why":"again","code":"selection_stale","message":"stale"}',
], tapeFile)
writefile(['1', 'TWO', '3'], dir .. '/a.go')

def Kinds(): list<string>
  return mapnew(replay.Session().tl.frames, (_, f) => f.kind)
enddef

ui.Open('20261004-1530-kinds')
t.WaitFor((): bool => replay.Active(), 'the tape to open')
t.Equal(['look', 'edit'], Kinds(), 'failure is left out at the start')
t.Equal({failure: 2}, replay.Session().hidden, 'and the server says how many')
t.True(getwinvar(replay.Session().win, '&statusline') =~# '隠している: failure (2)', 'the status line says what is left out')
t.Equal(['look', 'edit', 'external'], ui.Kinds(), 'the default kinds')

# tl, te, tx and tf are local to srwr's buffers
for key in ['tl', 'te', 'tx', 'tf']
  for w in [replay.Session().win, replay.Session().sidebar]
    t.Equal(1, win_execute(w, 'echo maparg(' .. string(key) .. ', "n", 0, 1).buffer')->trim()->str2nr(), key .. ' is local in window ' .. w)
  endfor
endfor

# Go to the replace (seq 4) and turn failure on: the tape opens again, and the replace is still on the screen.
replay.Goto(1)
win_gotoid(replay.Session().win)
execute 'normal tf'
t.WaitFor((): bool => replay.Active() && len(replay.Session().tl.frames) == 4, 'the tape to open again with failure')
t.Equal(['look', 'failure', 'edit', 'failure'], Kinds(), 'failure is in, in the order recorded')
t.Equal(2, replay.Session().index, 'the replace (seq 4) is still the frame on the screen')
t.Equal({}, replay.Session().hidden, 'nothing is left out now')
t.Equal(['look', 'edit', 'external', 'failure'], ui.Kinds(), 'the kinds, in order')
replay.Goto(3)
t.True(getbufline(replay.Session().buf, 1, 1)[0] =~# '✖ edit', 'the second failure')
replay.Goto(1)
t.Equal('invalid_range', get(replay.Session().tl.frames[1], 'code', ''), 'the first failure')

# Turn look and failure off: only the replace is left.
ui.Toggle('look')
t.WaitFor((): bool => replay.Active() && len(replay.Session().tl.frames) == 3, 'the tape to open again without look')
ui.Toggle('failure')
t.WaitFor((): bool => replay.Active() && len(replay.Session().tl.frames) == 1, 'the tape to open again with the replace only')
t.Equal(['edit'], Kinds(), 'edit only')
t.Equal(0, replay.Session().index, 'the replace is on the screen')
t.Equal({look: 1, failure: 2}, replay.Session().hidden, 'what is left out')
t.Equal(1, len(getbufline(sidebar.Buf(), 1, '$')), 'the list has one row')
t.True(getbufline(sidebar.Buf(), 1)[0] =~# '^● *1 edit', 'numbered 1 again: ' .. getbufline(sidebar.Buf(), 1)[0])

# Nothing shown.
ui.Toggle('edit')
t.WaitFor((): bool => replay.Active() && len(replay.Session().tl.frames) == 0, 'the tape to open again with nothing')
t.Equal(['表示するコマがありません'], getbufline(replay.Session().buf, 1, '$'), 'it says there is nothing to show')

# An unknown kind changes nothing.
const before = ui.Kinds()
ui.Toggle('final')
t.Equal(before, ui.Kinds(), 'an unknown kind')
replay.Close()
ui.Toggle('look')
ui.Toggle('edit')
t.Equal(['look', 'edit', 'external'], ui.Kinds(), 'without a tape open the kinds are only remembered')
t.Finish()

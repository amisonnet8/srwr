vim9script

import autoload './buf.vim'
import autoload './lang.vim'
import autoload './paint.vim'

# A diff frame (external, final): two windows side by side, before on the left (blue) and after on the right (orange),
# only the changed lines painted. Vim's own Diff* colors are taken away while a diff frame is shown.

const DIFF_GROUPS = ['DiffAdd', 'DiffChange', 'DiffDelete', 'DiffText']
var savedColors: list<dict<any>> = []
var taken = false

# Label is the heading of a diff frame.
export def Label(f: dict<any>): string
  const name = fnamemodify(f.file, ':t')
  if f.kind ==# 'final'
    const gone = get(f, 'deleted', false)
    return lang.Pick(
      '⚠ Changed after recording (' .. (gone ? 'no longer exists' : 'diff from current file') .. '): ' .. name,
      '⚠ 録画のあとで変更（' .. (gone ? '今は存在しない' : '今のファイルとの差分') .. '）：' .. name)
  endif
  if f.kind ==# 'replace'
    return lang.Pick('⚠ replace: ' .. name, '⚠ replace：' .. name)
  endif
  const deleted = get(f, 'deleted', false)
  return lang.Pick(
    '⚠ Changed outside srwr' .. (deleted ? ' (deleted)' : '') .. ': ' .. name,
    '⚠ srwrの外で変更' .. (deleted ? '（削除）' : '') .. '：' .. name)
enddef

# Active tells whether the right-hand window of the session exists.
export def Active(s: dict<any>): bool
  return get(s, 'diffWin', 0) > 0 && win_id2win(s.diffWin) > 0
enddef

def TakeAwayDiffColors()
  if taken
    return
  endif
  taken = true
  savedColors = []
  for g in DIFF_GROUPS
    savedColors += hlget(g)
    hlset([{name: g, cleared: true}])
  endfor
enddef

def GiveBackDiffColors()
  if !taken
    return
  endif
  taken = false
  hlset(savedColors)
  savedColors = []
enddef

# FoldText is the text of a folded stretch of unchanged lines: "+-- 17 lines: the first line". It is Vim's own text, but the
# word after the number ("lines" or "行" by the language of the Vim) is the one of the language of srwr, like every word of srwr.
export def FoldText(): string
  return substitute(foldtext(), '^\(+-*\s*\d\+\) \S\+:', '\1 ' .. lang.Pick('lines', '行') .. ':', '')
enddef

# Changed returns the lines of the window's buffer that differ from the other side.
def Changed(win: number): list<number>
  win_execute(win, 'diffupdate')
  win_execute(win, 'legacy let g:srwr_changed = filter(range(1, line("$")), "diff_hlID(v:val, 1) != 0")')
  const out: list<number> = get(g:, 'srwr_changed', [])
  unlet! g:srwr_changed
  return out
enddef

# ScrollNearTop puts line lnum about 30% from the top of window w, with the cursor on it.
def ScrollNearTop(w: number, lnum: number)
  const top = max([lnum - float2nr(winheight(w) * 0.3), 1])
  win_execute(w, 'call winrestview({topline: ' .. top .. ', lnum: ' .. lnum .. ', col: 1})')
enddef

# InsertBands puts `rows` above each of the lines `starts` (1-based, top to bottom, of the lines as they are).
def InsertBands(lines: list<string>, starts: list<number>, rows: list<string>): list<string>
  var out = copy(lines)
  for i in reverse(range(len(starts)))
    const at = min([max([starts[i], 1]), len(out) + 1])
    out = (at > 1 ? out[0 : at - 2] : []) + rows + out[at - 1 : ]
  endfor
  return out
enddef

# BandRows is the first row of each band after InsertBands: block i is pushed down by the i bands above it.
def BandRows(starts: list<number>, n: number): list<number>
  return mapnew(starts, (i, st) => max([st, 1]) + i * n)
enddef

# Hunks of a replace frame: where each block of changed lines starts on each side. A server that sends none is taken to have
# one block, at the top.
def Starts(f: dict<any>, key: string): list<number>
  const hunks: list<dict<any>> = get(f, 'hunks', [])
  return empty(hunks) ? [1] : mapnew(hunks, (_, h) => h[key])
enddef

# Enter shows frame f as a diff. It returns true when it had to create the right-hand window. A replace frame has a band of rows
# above each block of changed lines, the same number on both sides: its why on the right (orange), empty rows on the left (blue).
export def Enter(s: dict<any>, f: dict<any>, before: string, after: string, band: list<string> = []): bool
  var created = false
  if !Active(s)
    win_gotoid(s.win)
    rightbelow vnew
    s.diffWin = win_getid()
    s.diffBuf = bufnr()
    buf.SetupReadonly()
    win_gotoid(s.win)
    created = true
  endif
  const nameBase = 'srwr://' .. s.tape
  buf.Name(s.buf, s.win, nameBase .. '/before/' .. f.file)
  buf.Name(s.diffBuf, s.diffWin, nameBase .. '/after/' .. f.file)
  # Lines are cut from a buffer in diff mode only after diff mode is off: Vim 9.0.0784 fails with E315 (ml_get: Invalid lnum)
  # when a frame with fewer lines follows one with more.
  for w in [s.win, s.diffWin]
    win_execute(w, 'diffoff')
  endfor
  const n = len(band)
  const startsL = n > 0 ? Starts(f, 'beforeStart') : []
  const startsR = n > 0 ? Starts(f, 'afterStart') : []
  const bandsL = BandRows(startsL, n)
  const bandsR = BandRows(startsR, n)
  buf.SetLines(s.buf, InsertBands(buf.Lines(before), startsL, repeat([''], n)))
  buf.SetLines(s.diffBuf, InsertBands(buf.Lines(after), startsR, band))
  TakeAwayDiffColors()
  for w in [s.win, s.diffWin]
    win_execute(w, n > 0 ? 'setlocal nonumber' : 'setlocal number')
    win_execute(w, 'diffoff | diffthis | setlocal fillchars+=diff:\ ')
    win_execute(w, 'setlocal foldtext=srwr#diff#FoldText()')
  endfor
  var firsts: dict<number> = {}
  for [w, b, tone, bands] in [[s.win, s.buf, 'select', bandsL], [s.diffWin, s.diffBuf, 'replace', bandsR]]
    paint.Clear(b)
    # The band rows differ between the sides (the why against empty rows), so they are not changed lines.
    const lines = filter(Changed(w), (_, l) => empty(filter(copy(bands), (_, at) => l >= at && l < at + n)))
    for lnum in lines
      paint.Line(b, lnum, 'srwr_' .. tone)
    endfor
    for at in bands
      for k in range(n)
        paint.Line(b, at + k, 'srwr_why_' .. tone)
      endfor
    endfor
    if n > 0
      # The bands make 'number' wrong: the file's own numbers are drawn instead.
      paint.NumbersAt(b, bands, n)
    endif
    # What goes about 30% from the top of the window: the first band (the why is read before the change), or the first changed line.
    firsts[string(w)] = !empty(bands) ? bands[0] : (empty(lines) ? 0 : lines[0])
  endfor
  # From the top of the file, a change that is far down needs scrolling. A side without a changed line (only added, only
  # removed) follows the other side.
  const any = max([firsts[string(s.win)], firsts[string(s.diffWin)]])
  for w in [s.win, s.diffWin]
    const own = firsts[string(w)]
    ScrollNearTop(w, own > 0 ? own : max([any, 1]))
  endfor
  return created
enddef

# Leave goes back to a single window.
export def Leave(s: dict<any>)
  if get(s, 'diffWin', 0) == 0
    return
  endif
  const w = s.diffWin
  const b = s.diffBuf
  s.diffWin = 0
  s.diffBuf = 0
  if win_id2win(s.win) > 0
    win_execute(s.win, 'diffoff')
  endif
  GiveBackDiffColors()
  if win_id2win(w) > 0
    win_execute(w, 'silent! close')
  endif
  if bufexists(b)
    execute 'silent! bwipeout! ' .. b
  endif
enddef

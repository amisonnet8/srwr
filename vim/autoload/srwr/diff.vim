vim9script

import autoload './buf.vim'
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
    return '⚠ 録画のあとで変更（' .. (get(f, 'deleted', false) ? '今は存在しない' : '今のファイルとの差分') .. '）：' .. name
  endif
  return '⚠ srwrの外で変更' .. (get(f, 'deleted', false) ? '（削除）' : '') .. '：' .. name
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

# FoldText is the text of a folded stretch of unchanged lines: "+-- 17 行: the first line". It is Vim's own text, but the
# word after the number ("lines" or "行" by the language of the Vim) is always the Japanese one, like every word of srwr.
export def FoldText(): string
  return substitute(foldtext(), '^\(+-*\s*\d\+\) \S\+:', '\1 行:', '')
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

# Enter shows frame f as a diff. It returns true when it had to create the right-hand window.
export def Enter(s: dict<any>, f: dict<any>, before: string, after: string): bool
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
  buf.SetLines(s.buf, buf.Lines(before))
  buf.SetLines(s.diffBuf, buf.Lines(after))
  TakeAwayDiffColors()
  for w in [s.win, s.diffWin]
    win_execute(w, 'setlocal number')
    win_execute(w, 'diffoff | diffthis | setlocal fillchars+=diff:\ ')
    win_execute(w, 'setlocal foldtext=srwr#diff#FoldText()')
  endfor
  var firsts: dict<number> = {}
  for [w, b, tone] in [[s.win, s.buf, 'select'], [s.diffWin, s.diffBuf, 'replace']]
    paint.Clear(b)
    const lines = Changed(w)
    for lnum in lines
      paint.Line(b, lnum, 'srwr_' .. tone)
    endfor
    firsts[string(w)] = empty(lines) ? 0 : lines[0]
  endfor
  # The first changed line goes about 30% from the top of the window (from the top of the file, a change that is
  # far down needs scrolling). A side without a changed line (only added, only removed) follows the other side.
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

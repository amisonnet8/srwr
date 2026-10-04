vim9script

import autoload './buf.vim'
import autoload './diff.vim'
import autoload './hl.vim'
import autoload './lang.vim'
import autoload './paint.vim'
import autoload './sidebar.vim'
import autoload './timeline.vim'

# Replay: one tab with the operation list at the left and the frame at the right. Live uses the same screen:
# the list only grows (Append), and the step to a frame is the same Goto.

const BANNER_PREFIX = '◆ '
def CloseHint(): string
  return lang.Pick('(Close: q in the list on the left, or :SrwrClose)', '（閉じる：左の一覧で q、または :SrwrClose）')
enddef

# The session being shown; {} when none.
var s: dict<any> = {}

def NoLiveClose()
enddef

var OnLiveClose: func() = NoLiveClose

export def SetOnLiveClose(F: func())
  OnLiveClose = F
enddef

def NoToggle(_kind: string)
enddef

var OnToggle: func(string) = NoToggle

# SetOnToggle registers what the keys ts, tr, te and tf call (the kinds of frames are chosen where the server is spoken to).
export def SetOnToggle(F: func(string))
  OnToggle = F
enddef

def Toggle(kind: string)
  OnToggle(kind)
enddef

export def Active(): bool
  return !empty(s)
enddef

export def Session(): dict<any>
  return s
enddef

# --- the why rows ---

# Wrap cuts the why into rows of at most `width` display cells. The first row starts with the mark, the
# others are indented under it. A why is shown in full.
export def Wrap(text: string, width: number): list<string>
  const room = max([width - strdisplaywidth(BANNER_PREFIX), 8])
  var rows: list<string> = []
  for para in split(text, "\n", true)
    var rest = para
    while true
      var n = strcharlen(rest)
      while n > 1 && strdisplaywidth(strcharpart(rest, 0, n)) > room
        n -= 1
      endwhile
      add(rows, (empty(rows) ? BANNER_PREFIX : repeat(' ', strdisplaywidth(BANNER_PREFIX))) .. strcharpart(rest, 0, n))
      rest = strcharpart(rest, n)
      if rest ==# ''
        break
      endif
    endwhile
  endfor
  return rows
enddef

# BannerRows are the why rows of a frame: none when it has no why.
export def BannerRows(f: dict<any>, width: number): list<string>
  const why = timeline.Why(f)
  return why ==# '' ? [] : Wrap(why, width)
enddef

# BannerAt is the line (1-based) the why rows are put in front of: the first line of the range, kept inside the content.
export def BannerAt(start: number, nlines: number): number
  return min([max([start, 1]), nlines + 1])
enddef

# WithBanner puts the rows in front of line `start` of lines.
export def WithBanner(lines: list<string>, start: number, rows: list<string>): list<string>
  const at = BannerAt(start, len(lines))
  # lines[0 : -1] is the whole list, not nothing, so a banner at the top is handled apart.
  return (at > 1 ? lines[0 : at - 2] : []) + rows + lines[at - 1 :]
enddef

# --- scrolling ---

# RevealTop decides where the window must scroll so that the lines first..last (the why rows and the first line of the
# range) are all on the screen. It returns -1 when they already are. Otherwise it returns the line to put at the top: the first line
# of the block in the middle of the window (what `zz` does), but never so low that the window shows empty lines after the
# end of the content; a block taller than the window starts at the top. All numbers are line numbers of the buffer with
# the why rows put in.
export def RevealTop(topline: number, height: number, first: number, last: number, total: number): number
  if height < 1 || (topline <= first && last <= topline + height - 1)
    return -1
  endif
  if last - first + 1 >= height
    return max([first, 1])
  endif
  var top = first - (height - 1) / 2
  top = min([top, max([total - height + 1, 1])])
  return max([min([top, first]), 1])
enddef

def Reveal(first: number, last: number)
  const info = getwininfo(s.win)
  if empty(info)
    return
  endif
  const top = RevealTop(info[0].topline, info[0].height, first, last, line('$', s.win))
  if top >= 0
    win_execute(s.win, 'call winrestview({topline: ' .. top .. '})')
  endif
enddef

# --- one frame ---

def TextWidth(): number
  const info = getwininfo(s.win)
  # The number column (virtual text with why rows, 'number' without) is 4 cells; do not depend on which one is on now.
  return empty(info) ? 80 : info[0].width - 4
enddef

# ShowFile points the window at the buffer name of file.
def ShowFile(file: string)
  if s.file !=# file
    buf.Name(s.buf, s.win, 'srwr://' .. s.tape .. '/' .. file)
    s.file = file
    # The filetype changed, and a filetype plugin may have mapped the same keys (markdown maps ]] and [[): ours go last.
    MapKeys(s.win)
  endif
enddef

# Render writes the content with the why rows above the frame's range, and paints the range.
def Render(f: dict<any>, content: any)
  ShowFile(f.file)
  const rows = BannerRows(f, TextWidth())
  const lines = buf.Lines(type(content) == v:t_string ? content : '')
  buf.SetLines(s.buf, WithBanner(lines, f.range.start, rows))
  const at = BannerAt(f.range.start, len(lines))
  const tone = timeline.Tone(f)
  for k in range(len(rows))
    paint.Line(s.buf, at + k, 'srwr_why_' .. tone)
  endfor
  paint.Range(s.buf, {start: at, end: at + (f.range.end - f.range.start)}, len(rows), tone)
  # With why rows 'number' would count them: show the file's own numbers instead. Set every frame (the window keeps it).
  setwinvar(s.win, '&number', empty(rows))
  if !empty(rows)
    paint.Numbers(s.buf, at, len(rows))
  endif
  # What must be on the screen: the why rows and the first line of the range (a long range may run past the window).
  const first = at
  const last = at + len(rows)
  # Every frame starts from the top of the content, so a frame whose range is near the top looks the same whatever the
  # last frame showed; the window scrolls only when the why rows and the range do not fit from there.
  win_execute(s.win, 'call winrestview({topline: 1, lnum: 1, col: 1, leftcol: 0})')
  win_execute(s.win, 'call cursor(' .. min([at + len(rows), line('$', s.win)]) .. ', 1)')
  Reveal(first, max([last, first]))
enddef

# --- going to a frame ---

# Goto shows frame i.
export def Goto(i: number)
  if !Active() || timeline.Len(s.tl) == 0
    return
  endif
  s.index = min([max([i, 0]), timeline.Len(s.tl) - 1])
  s.wanted = s.index
  const f: dict<any> = s.tl.frames[s.index]
  if timeline.IsDiff(f)
    ShowDiff(f)
  elseif f.kind ==# 'failure'
    LeaveDiff()
    RenderFailure(f)
  else
    LeaveDiff()
    Render(f, timeline.ContentAt(s.tl, f.file, s.index))
  endif
  sidebar.Mark(s.index)
  UpdateStatus()
enddef

# RenderFailure shows a failure: it has no file to open, so the buffer explains it, with the first row in red.
def RenderFailure(f: dict<any>)
  ShowFile('failure' .. f.index)
  # The explanation is not code: without this the filetype of the last file (its colors for select, range...) would stay.
  win_execute(s.win, 'setlocal filetype= syntax=')
  buf.SetLines(s.buf, timeline.FailureLines(f))
  paint.Line(s.buf, 1, 'srwr_why_failure')
  setwinvar(s.win, '&number', 0)
  win_execute(s.win, 'call winrestview({topline: 1, lnum: 1, col: 1, leftcol: 0})')
enddef

# SubBand is the band of a sub frame: its why wrapped to the width of the right-hand window and centered in it, and at
# least one row. The right-hand window is half of this one when it is not there yet.
def SubBand(f: dict<any>): list<string>
  const width = diff.Active(s) ? WinWidth(s.diffWin) : (WinWidth(s.win) - 1) / 2
  const room = max([width - 4, 20])
  const rows = BannerRows(f, room)
  return mapnew(empty(rows) ? [BANNER_PREFIX] : rows, (_, row) => repeat(' ', max([(room - strdisplaywidth(row)) / 2, 0])) .. row)
enddef

def ShowDiff(f: dict<any>)
  paint.Clear(s.buf)
  s.file = ''
  diff.Enter(s, f, f.before, f.after, f.kind ==# 'sub' ? SubBand(f) : [])
  MapKeys(s.win)
  MapKeys(s.diffWin)
enddef

def LeaveDiff()
  if diff.Active(s)
    diff.Leave(s)
    s.file = ''
  endif
enddef

export def StepForward()
  if Active()
    Goto(s.index + 1)
  endif
enddef

export def StepBack()
  if Active()
    Goto(s.index - 1)
  endif
enddef

# Jump is used by the operation list.
export def Jump(i: number)
  Goto(i)
enddef

# --- status line ---

# Behind is how many frames arrived after the one on the screen (live only).
export def Behind(): number
  return timeline.Len(s.tl) - 1 - s.index
enddef

# StatusParts is the status line as pieces [text, style]: style is '' (plain), 'dim' (a button that cannot be used now)
# or 'new' (live, frames waiting); 'cut' is not text but the place where a line that is too long is cut.
# `room` is the width the line has; the close hint is left out when all of the line
# with it does not fit, so that the live mark is never cut off. `lead` is what comes before it in this window.
export def StatusParts(index: number, total: number, live: bool, where: string, room: number, lead: string = '', hidden: string = ''): list<list<string>>
  # What the server left out goes at the right end ('right'): "hidden: failure (2)".
  const rightEnd: list<list<string>> = hidden ==# '' ? [] : [['', 'right'], ['  ' .. hidden, '']]
  if live && total == 0
    return [[lang.Pick("srwr  ● LIVE  (waiting for the AI's operations)", 'srwr  ● LIVE  （AI の操作を待っています）'), '']] + rightEnd
  endif
  const behind = total - 1 - index
  var parts: list<list<string>> = [
    ['srwr  ' .. max([index + 1, 0]) .. '/' .. total .. '  ', ''],
    ['[[ ' .. lang.Pick('Back', '戻る'), index > 0 ? '' : 'dim'],
    ['  ', ''],
    [']] ' .. lang.Pick('Forward', '進む'), index < total - 1 ? '' : 'dim'],
  ]
  if live
    if behind == 0
      add(parts, ['  ● LIVE', ''])
    else
      add(parts, ['  ', ''])
      add(parts, [lang.Pick(' L: Back to LIVE (' .. behind .. ' new) ', ' L：LIVE に戻る（新着 ' .. behind .. '） '), 'new'])
    endif
  endif
  if where !=# ''
    # When the line is too long for the window, the file and range are what is cut (the buttons and the position stay).
    add(parts, ['', 'cut'])
    add(parts, ['  ' .. where, ''])
  endif
  if live
    var withHint = copy(parts)
    add(withHint, ['  ' .. CloseHint(), ''])
    if strdisplaywidth(lead .. join(mapnew(withHint + rightEnd, (_, p) => p[0]), '')) <= room
      return withHint + rightEnd
    endif
  endif
  return parts + rightEnd
enddef

# StatusString is StatusParts as a value for 'statusline'.
export def StatusString(parts: list<list<string>>, lead: string = ''): string
  var out = substitute(lead, '%', '%%', 'g')
  for [text, style] in parts
    const t = substitute(text, '%', '%%', 'g')
    if style ==# 'cut'
      out ..= '%<'
    elseif style ==# 'right'
      out ..= '%='
    elseif style ==# 'dim'
      out ..= '%#SrwrDim#' .. t .. '%*'
    elseif style ==# 'new'
      out ..= '%#SrwrWhyReplace#' .. t .. '%*'
    else
      out ..= t
    endif
  endfor
  return out
enddef

def Where(): string
  const n = timeline.Len(s.tl)
  if s.index >= 0 && s.index < n
    const f: dict<any> = s.tl.frames[s.index]
    return f.kind ==# 'failure' ? 'failure: ' .. get(f, 'tool', '') : f.file .. ':' .. timeline.FormatRange(f.range)
  endif
  return ''
enddef

# Status is the plain text of the status line: srwr  5/7  [[ 戻る  ]] 進む  text.go:37
export def Status(room: number = 1000): string
  if !Active()
    return ''
  endif
  return join(mapnew(StatusParts(s.index, timeline.Len(s.tl), s.live, Where(), room, '', timeline.HiddenText(s.hidden)), (_, p) => p[0]), '')
enddef

def WinWidth(w: number): number
  const info = getwininfo(w)
  return empty(info) ? 80 : info[0].width
enddef

def UpdateStatus()
  if !Active()
    return
  endif
  const n = timeline.Len(s.tl)
  if diff.Active(s)
    const f: dict<any> = s.tl.frames[s.index]
    const lead = lang.Pick('After  ', '後  ')
    setwinvar(s.win, '&statusline', StatusString([[lang.Pick('Before  ', '前  ') .. diff.Label(f), '']]))
    setwinvar(s.diffWin, '&statusline', StatusString(StatusParts(s.index, n, s.live, Where(), WinWidth(s.diffWin), lead, timeline.HiddenText(s.hidden)), lead))
  else
    setwinvar(s.win, '&statusline', StatusString(StatusParts(s.index, n, s.live, Where(), WinWidth(s.win), '', timeline.HiddenText(s.hidden))))
  endif
enddef

# --- keys ---

def MapKeys(win: number)
  for [lhs, fn] in [
      [']]', 'StepForward()'], ['<Right>', 'StepForward()'],
      ['[[', 'StepBack()'], ['<Left>', 'StepBack()'],
      ['q', 'Close()'],
      ['ts', "Toggle('select')"], ['tr', "Toggle('replace')"], ['te', "Toggle('external')"], ['tf', "Toggle('failure')"]]
    win_execute(win, 'nnoremap <buffer><silent><nowait> ' .. lhs .. ' <ScriptCmd>' .. fn .. '<CR>')
  endfor
  if s.live
    win_execute(win, 'nnoremap <buffer><silent><nowait> L <ScriptCmd>Latest()<CR>')
  endif
enddef

# --- live ---

# Append adds a frame that arrived (live/frame). A frame of another tape starts the list again. While following
# the newest frame it is shown at once; after a step back the screen stays and the arrivals are counted.
export def Append(tapeId: string, f: dict<any>)
  if !Active() || !s.live
    return
  endif
  if tapeId !=# s.tape
    s.tape = tapeId
    s.tl = timeline.New()
    s.hidden = {}
    s.index = -1
    s.wanted = -1
    LeaveDiff()
    s.file = ''
  endif
  const following = s.wanted == timeline.Len(s.tl) - 1
  timeline.Append(s.tl, f)
  sidebar.Fill(s.tl)
  if following
    s.wanted = timeline.Len(s.tl) - 1
    Goto(s.wanted)
  else
    sidebar.Mark(s.index)
    UpdateStatus()
  endif
enddef

# SetHidden is the count of the frames the server leaves out, which changes as frames of those kinds arrive (live/hidden).
export def SetHidden(tapeId: string, hidden: dict<any>)
  if !Active() || !s.live
    return
  endif
  if tapeId !=# s.tape
    s.tape = tapeId
    s.tl = timeline.New()
    s.index = -1
    s.wanted = -1
    LeaveDiff()
    s.file = ''
    sidebar.Fill(s.tl)
  endif
  s.hidden = hidden
  UpdateStatus()
enddef

# Latest goes back to the newest frame and follows again.
export def Latest()
  if Active() && s.live && timeline.Len(s.tl) > 0
    s.wanted = timeline.Len(s.tl) - 1
    Goto(s.wanted)
  endif
enddef

# --- opening and closing ---

# OpenLive starts the live view in a new tab: the replay screen, on a list that grows. The frames up to now
# (from live/start) are only listed; nothing is shown until a frame arrives or a step is made.
export def OpenLive(tapeId: any, frames: list<dict<any>>, root: string, hidden: dict<any> = {})
  Open(type(tapeId) == v:t_string ? tapeId : 'live', frames, root, true, hidden)
enddef

# Open starts the replay of a tape in a new tab. frames come from tape/open with withText.
# hidden is how many frames of each kind the server left out; `at` is the frame to start at (the first one by default).
export def Open(tapeId: string, frames: list<dict<any>>, root: string, live: bool = false, hidden: dict<any> = {}, at: number = 0)
  if Active()
    Close()
  endif
  hl.Setup()
  tabnew
  const tabId = tabpagenr()
  s = {
    tape: tapeId, tl: timeline.New(frames), root: root, index: -1, file: '',
    diffWin: 0, diffBuf: 0, win: win_getid(), buf: bufnr(), tab: tabId, live: live, wanted: live ? len(frames) - 1 : -1,
    hidden: hidden,
  }
  buf.SetupReadonly()
  silent keepalt file srwr://opening
  const sbwin = sidebar.Create(Jump)
  s.sidebar = sbwin
  MapKeys(s.win)
  MapKeys(sbwin)
  sidebar.Fill(s.tl)
  win_gotoid(s.win)
  augroup srwr_replay
    autocmd!
    autocmd WinClosed * OnWinClosed(expand('<amatch>'))
    autocmd VimResized,WinScrolled * UpdateStatus()
  augroup END
  if live
    buf.SetLines(s.buf, [lang.Pick("Live view (waiting for the AI's operations)", 'ライブ視聴中（AI の操作を待っています）')])
    s.index = len(frames) - 1
    sidebar.Mark(s.index)
    UpdateStatus()
  elseif timeline.Len(s.tl) > 0
    Goto(at)
  else
    buf.SetLines(s.buf, [lang.Pick('No frames to show', '表示するコマがありません')])
    UpdateStatus()
  endif
enddef

def OnWinClosed(id: string)
  if Active() && (id == string(s.win) || id == string(s.sidebar))
    Close()
  endif
enddef

# Close ends the replay: the tab and its buffers go away. Nothing is left behind.
export def Close()
  if !Active()
    return
  endif
  final closing = s
  s = {}
  if closing.live
    OnLiveClose()
  endif
  augroup srwr_replay
    autocmd!
  augroup END
  diff.Leave(closing)
  for w in [closing.win, closing.sidebar]
    if w > 0 && win_id2win(w) > 0
      win_execute(w, 'silent! close')
    endif
  endfor
  for b in [closing.buf, sidebar.Buf(), closing.diffBuf]
    if bufexists(b)
      execute 'silent! bwipeout! ' .. b
    endif
  endfor
enddef

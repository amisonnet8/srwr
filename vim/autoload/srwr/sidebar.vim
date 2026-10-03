vim9script

import autoload './buf.vim'
import autoload './timeline.vim'

# The operation list (left): frames in recorded order, numbered from 1. A colored dot tells the kind.

const WIDTH = 40
const KIND_LABEL = {select: 'select ', replace: 'replace', external: '外部変更', final: '録画後'}

var win = 0
var lbuf = 0
def NoJump(_i: number)
enddef

var JumpTo: func(number) = NoJump
var rowCount = 0

# Line is the text of one frame: " 5 ● replace text.go:37  why".
export def Line(f: dict<any>): string
  const desc = timeline.Why(f)
  return printf('%2d ● %s %s:%s', f.index + 1, KIND_LABEL[f.kind], timeline.Basename(f.file), timeline.FormatRange(f.range)) ..
    (desc !=# '' ? '  ' .. desc : '')
enddef

# Create opens the list as a window at the left of the tab. OnJump(i) is called for <CR> on the row of frame i.
export def Create(OnJump: func(number)): number
  JumpTo = OnJump
  topleft vnew
  win = win_getid()
  lbuf = bufnr()
  buf.SetupReadonly()
  setlocal winfixwidth nonumber norelativenumber nowrap nolist signcolumn=no foldcolumn=0 cursorline
  silent keepalt file srwr://operations
  execute 'vertical resize ' .. WIDTH
  nnoremap <buffer><silent><nowait> <CR> <ScriptCmd>Enter()<CR>
  return win
enddef

def Enter()
  const i = line('.') - 1
  if i >= 0 && i < rowCount
    JumpTo(i)
  endif
enddef

# DotType is the text property type of a frame's dot: select blue, replace orange, external and final purple.
export def DotType(kind: string): string
  return kind ==# 'select' ? 'srwr_dot_select' : kind ==# 'replace' ? 'srwr_dot_replace' : 'srwr_dot_external'
enddef

# Fill writes the frames of tl into the list buffer, and colors the dots.
export def Fill(tl: dict<any>)
  rowCount = len(tl.frames)
  buf.SetLines(lbuf, mapnew(tl.frames, (_, f) => Line(f)))
  for f in tl.frames
    prop_add(f.index + 1, 4, {bufnr: lbuf, type: DotType(f.kind), length: len('●')})
  endfor
enddef

# Mark highlights the line of frame `current` and puts the cursor on it.
export def Mark(current: number)
  prop_remove({type: 'srwr_current', bufnr: lbuf, all: true})
  if current < 0 || current >= rowCount
    return
  endif
  const text = getbufline(lbuf, current + 1)[0]
  if text !=# ''
    prop_add(current + 1, 1, {bufnr: lbuf, type: 'srwr_current', length: len(text)})
  endif
  prop_add(current + 1, 0, {bufnr: lbuf, type: 'srwr_current', text: repeat(' ', 200)})
  win_execute(win, 'cursor(' .. (current + 1) .. ', 1)')
enddef

export def Win(): number
  return win
enddef

export def Buf(): number
  return lbuf
enddef

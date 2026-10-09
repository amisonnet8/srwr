vim9script

import autoload './lang.vim'

# New wraps the frames of an opened tape (with before/after, i.e. withText) in a timeline.
export def New(frames: list<dict<any>> = []): dict<any>
  return {frames: copy(frames)}
enddef

# Append adds a frame that arrived by live/frame.
export def Append(tl: dict<any>, frame: dict<any>)
  add(tl.frames, frame)
enddef

export def Len(tl: dict<any>): number
  return len(tl.frames)
enddef

# IsDiff tells the frames shown as two windows, before and after: external and final.
export def IsDiff(frame: dict<any>): bool
  return frame.kind ==# 'external' || frame.kind ==# 'final'
enddef

# ContentAt is the content of file after frame i. A file no frame has touched yet is as it was
# before the first frame that touches it. v:null when no frame touches it.
export def ContentAt(tl: dict<any>, file: string, i: number): any
  var j = min([i, len(tl.frames) - 1])
  while j >= 0
    if tl.frames[j].file ==# file && tl.frames[j].kind !=# 'failure'
      return tl.frames[j].after
    endif
    j -= 1
  endwhile
  j = max([i + 1, 0])
  while j < len(tl.frames)
    if tl.frames[j].file ==# file && tl.frames[j].kind !=# 'failure'
      return tl.frames[j].before
    endif
    j += 1
  endwhile
  return v:null
enddef

export def Basename(file: string): string
  const i = strridx(file, '/')
  return i < 0 ? file : file[i + 1 :]
enddef

# FormatRange: an empty range reads "before 12" ("12の前").
export def FormatRange(r: dict<any>): string
  if r.end < r.start
    return lang.Pick('before ' .. r.start, r.start .. 'の前')
  endif
  return r.start == r.end ? string(r.start) : r.start .. '-' .. r.end
enddef

# Tone is the color of a frame (the names are those of the highlight groups): look is blue, everything that changes a file is orange, a failure is red.
export def Tone(f: dict<any>): string
  return f.kind ==# 'look' ? 'select' : f.kind ==# 'failure' ? 'failure' : 'replace'
enddef

# NearestBySeq is the index of the frame whose seq is nearest to seq (the earlier one on a tie), or -1 when there is no frame.
export def NearestBySeq(tl: dict<any>, seq: number): number
  var best = -1
  for f in tl.frames
    const d = abs(get(f, 'seq', 0) - seq)
    if best < 0 || d < abs(get(tl.frames[best], 'seq', 0) - seq)
      best = f.index
    endif
  endfor
  return best
enddef

# HiddenText says what the server left out: "hidden: failure (2)", or '' when nothing.
export def HiddenText(hidden: dict<any>): string
  var parts: list<string> = []
  for kind in ['look', 'edit', 'external', 'failure']
    if get(hidden, kind, 0) > 0
      add(parts, kind .. ' (' .. hidden[kind] .. ')')
    endif
  endfor
  return empty(parts) ? '' : lang.Pick('hidden: ', '隠している: ') .. join(parts, ', ')
enddef

# FailureLines are the rows of a failure frame, which has no file to show: a red first row, the message, and what is known of the call.
export def FailureLines(f: dict<any>): list<string>
  const tool = get(f, 'tool', '')
  const why = Why(f)
  const range = tool ==# 'look' ? lang.Pick('lines ' .. FormatRange(f.range), FormatRange(f.range) .. ' 行') : '-'
  return [
    lang.Pick('✖ ' .. tool .. ' failed  (' .. get(f, 'code', '') .. ')', '✖ ' .. tool .. ' が失敗しました  (' .. get(f, 'code', '') .. ')'),
    get(f, 'message', ''),
    '',
    'why    ' .. (why ==# '' ? lang.Pick('(none)', '(なし)') : why),
    'tool   ' .. tool,
    'range  ' .. range,
    'file   ' .. (f.file ==# '' ? '(not shown)' : f.file),
  ]
enddef

# Why is the why of a frame, or '' when there is none.
export def Why(f: dict<any>): string
  return type(f.why) == v:t_string ? f.why : ''
enddef

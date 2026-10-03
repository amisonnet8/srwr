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

export def IsDiff(frame: dict<any>): bool
  return frame.kind ==# 'external' || frame.kind ==# 'final'
enddef

# ContentAt is the content of file after frame i. A file no frame has touched yet is as it was
# before the first frame that touches it. v:null when no frame touches it.
export def ContentAt(tl: dict<any>, file: string, i: number): any
  var j = min([i, len(tl.frames) - 1])
  while j >= 0
    if tl.frames[j].file ==# file
      return tl.frames[j].after
    endif
    j -= 1
  endwhile
  j = max([i + 1, 0])
  while j < len(tl.frames)
    if tl.frames[j].file ==# file
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

# Tone is the color of a frame: select is blue, everything that changes a file is orange.
export def Tone(f: dict<any>): string
  return f.kind ==# 'select' ? 'select' : 'replace'
enddef

# Why is the why of a frame, or '' when there is none.
export def Why(f: dict<any>): string
  return type(f.why) == v:t_string ? f.why : ''
enddef

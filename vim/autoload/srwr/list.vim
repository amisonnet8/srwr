vim9script

import autoload './buf.vim'
import autoload './lang.vim'

# The list of tapes (srwr://tapes): newest first. <CR> opens one, q closes the list.

const HEADER_JA = '開始時刻          更新時刻            操作  ファイル  テープ'

export def Header(): string
  return lang.Ja() ? HEADER_JA : printf('%-16s  %-16s  %4s  %8s  %s', 'Started', 'Updated', 'Ops', 'Files', 'Tape')
enddef

def NoOpen(_id: string)
enddef

var OnOpen: func(string) = NoOpen
var ids: list<string> = []
var listBuf = 0

# DaysFromCivil is the number of days from 1970-01-01 to the date (proleptic Gregorian; the algorithm of Howard Hinnant).
def DaysFromCivil(year: number, month: number, day: number): number
  const y = month <= 2 ? year - 1 : year
  const era = y / 400
  const yoe = y - era * 400
  const doy = (153 * (month + (month > 2 ? -3 : 9)) + 2) / 5 + day - 1
  const doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
  return era * 146097 + doe - 719468
enddef

# Time shows an RFC 3339 time as "2026-09-30 09:54" in the time zone of the machine (TZ). The tape holds UTC; a time with
# an offset ("+09:00", from older tapes) is the same moment. Text that is not a time is shown as it is.
export def Time(rfc3339: any): string
  if type(rfc3339) != v:t_string
    return ''
  endif
  const m = matchlist(rfc3339, '^\(\d\{4}\)-\(\d\d\)-\(\d\d\)T\(\d\d\):\(\d\d\):\(\d\d\)\%(\.\d\+\)\=\(Z\|[+-]\d\d:\d\d\)$')
  if empty(m)
    return rfc3339
  endif
  var offset = 0
  if m[7] !=# 'Z'
    offset = (str2nr(m[7][1 : 2]) * 3600 + str2nr(m[7][4 : 5]) * 60) * (m[7][0] ==# '-' ? -1 : 1)
  endif
  const epoch = DaysFromCivil(str2nr(m[1]), str2nr(m[2]), str2nr(m[3])) * 86400 + str2nr(m[4]) * 3600 + str2nr(m[5]) * 60 + str2nr(m[6]) - offset
  return strftime('%Y-%m-%d %H:%M', epoch)
enddef

# Line is one row of the list.
export def Line(t: dict<any>): string
  return printf('%-16s  %-16s  %4d  %8d  %s', Time(get(t, 'startedAt', '')), Time(get(t, 'updatedAt', '')), t.ops, len(t.files), t.tapeId)
enddef

# Show opens the list in a new tab. Open(tapeId) is called for <CR> on a row.
export def Show(tapes: list<dict<any>>, Open: func(string))
  OnOpen = Open
  ids = mapnew(tapes, (_, t) => t.tapeId)
  tabnew
  listBuf = bufnr()
  buf.SetupReadonly()
  setlocal nonumber cursorline
  silent keepalt file srwr://tapes
  buf.SetLines(listBuf, empty(tapes) ? [lang.Pick('No tapes (in .srwr/tapes/ of this workspace)', 'テープがありません（この作業場の .srwr/tapes/）')] : [Header()] + mapnew(tapes, (_, t) => Line(t)))
  nnoremap <buffer><silent><nowait> <CR> <ScriptCmd>Enter()<CR>
  nnoremap <buffer><silent><nowait> q <ScriptCmd>Close()<CR>
  if !empty(tapes)
    cursor(2, 1)
  endif
enddef

def Enter()
  const n = line('.') - 2
  if n >= 0 && n < len(ids)
    OnOpen(ids[n])
  endif
enddef

export def Close()
  if listBuf > 0 && bufexists(listBuf)
    execute 'silent! bwipeout! ' .. listBuf
  endif
  listBuf = 0
enddef

export def Active(): bool
  return listBuf > 0 && bufexists(listBuf)
enddef

export def Buf(): number
  return listBuf
enddef

vim9script

import autoload './buf.vim'

# The list of tapes (srwr://tapes): newest first. <CR> opens one, q closes the list.

const HEADER = '開始時刻          更新時刻            操作  ファイル  テープ'

def NoOpen(_id: string)
enddef

var OnOpen: func(string) = NoOpen
var ids: list<string> = []
var listBuf = 0

# Time shows an RFC 3339 time as "2026-09-30 00:54" (as recorded, in the zone it was written in).
export def Time(rfc3339: any): string
  if type(rfc3339) != v:t_string
    return ''
  endif
  return substitute(rfc3339, '^\(\d\{4}-\d\d-\d\d\)T\(\d\d:\d\d\).*$', '\1 \2', '')
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
  buf.SetLines(listBuf, empty(tapes) ? ['テープがありません（この作業場の .srwr/tapes/）'] : [HEADER] + mapnew(tapes, (_, t) => Line(t)))
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

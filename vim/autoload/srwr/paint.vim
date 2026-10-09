vim9script

# Line paints the whole row of line lnum: the text, and (so an empty or short line shows it too) the rest of the row.
export def Line(buf: number, lnum: number, type: string)
  const info = getbufinfo(buf)
  if empty(info) || lnum < 1 || lnum > info[0].linecount
    return
  endif
  const text = getbufline(buf, lnum)[0]
  if text !=# ''
    prop_add(lnum, 1, {bufnr: buf, type: type, length: len(text)})
  endif
  prop_add(lnum, 0, {bufnr: buf, type: type, text: repeat(' ', 300)})
enddef

# Range paints lines r.start..r.end (moved down by `shift`) in the tone ('select' or 'replace').
# An empty range paints nothing.
export def Range(buf: number, r: dict<any>, shift: number, tone: string)
  for lnum in range(r.start + shift, r.end + shift)
    Line(buf, lnum, 'srwr_' .. tone)
  endfor
enddef

# Clear removes everything painted in the buffer.
export def Clear(buf: number)
  const info = getbufinfo(buf)
  if !empty(info)
    prop_clear(1, max([info[0].linecount, 1]), {bufnr: buf})
  endif
enddef

# NumberLabels is the text at the start of each of `total` rows, of which `rows` why rows were put in at line `at`
# (WithBanner clamps `at`): the file's own line number, right-aligned, and blank space for a why row.
export def NumberLabels(total: number, at: number, rows: number): list<string>
  const start = min([max([at, 1]), total - rows + 1])
  const width = max([3, len(string(total - rows))])
  var out: list<string> = []
  for lnum in range(1, total)
    if lnum >= start && lnum < start + rows
      add(out, repeat(' ', width + 1))
    else
      const n = lnum > start ? lnum - rows : lnum
      add(out, repeat(' ', width - len(string(n))) .. n .. ' ')
    endif
  endfor
  return out
enddef

# Numbers writes those labels as virtual text at the start of every row (the buffer has why rows in it, so
# 'number' would count them).
export def Numbers(buf: number, at: number, rows: number)
  const info = getbufinfo(buf)
  if empty(info)
    return
  endif
  var lnum = 1
  for label in NumberLabels(info[0].linecount, at, rows)
    prop_add(lnum, 1, {bufnr: buf, type: 'srwr_num', text: label})
    lnum += 1
  endfor
enddef

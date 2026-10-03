vim9script

# Lines splits a text the way the server counts lines: "" is no lines and a final "\n" does not start another one.
export def Lines(text: any): list<string>
  if type(text) != v:t_string || text ==# ''
    return []
  endif
  return split(text[-1 : ] ==# "\n" ? text[0 : -2] : text, "\n", true)
enddef

# SetupReadonly: the options of every buffer the plugin shows.
export def SetupReadonly()
  setlocal buftype=nofile bufhidden=wipe noswapfile nomodifiable nobuflisted undolevels=-1
  setlocal number nowrap signcolumn=no
enddef

# SetLines replaces the whole content of a read-only buffer, and drops the text properties of the old content.
export def SetLines(buf: number, lines: list<string>)
  setbufvar(buf, '&modifiable', 1)
  const last = max([getbufinfo(buf)[0].linecount, 1])
  prop_clear(1, last, {bufnr: buf})
  # Overwrite first and cut the rest after: deleting every line would print "--No lines in buffer--".
  const want = empty(lines) ? [''] : lines
  setbufline(buf, 1, want)
  if last > len(want)
    deletebufline(buf, len(want) + 1, '$')
  endif
  setbufvar(buf, '&modifiable', 0)
enddef

# Name gives the buffer of window `win` a new name. The old name is left behind in a hidden buffer; it is wiped.
export def Name(buf: number, win: number, name: string)
  const old = bufname(buf)
  if old ==# name
    return
  endif
  win_execute(win, 'silent keepalt file ' .. fnameescape(name))
  win_execute(win, 'silent! filetype detect')
  if old !=# ''
    const stale = bufnr('^' .. escape(old, '\*.[]^$~') .. '$')
    if stale > 0 && stale != buf
      execute 'silent! bwipeout! ' .. stale
    endif
  endif
enddef

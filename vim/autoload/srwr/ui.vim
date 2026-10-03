vim9script

import autoload './list.vim'
import autoload './live.vim'
import autoload './replay.vim'
import autoload './server.vim'

# The entry of every command.

def Warn(msg: string)
  echohl ErrorMsg
  echomsg 'srwr: ' .. msg
  echohl None
enddef

# TapeId accepts a tape id or a path to a .tape.jsonl file.
export def TapeId(arg: string): string
  return substitute(fnamemodify(trim(arg), ':t'), '\.tape\.jsonl$', '', '')
enddef

# ShowList lists the tapes of the workspace; <CR> opens one.
def ShowList(root: string)
  server.Start(root, (_, err) => {
    if err != v:null
      Warn(err.message)
      return
    endif
    server.Request('tapes/list', {}, (res, err2) => {
      if err2 != v:null
        Warn(err2.message)
        return
      endif
      list.Show(res.tapes, (id) => Open(id))
    })
  })
enddef

export def Open(arg: string)
  const tape = TapeId(arg)
  const root = getcwd()
  if tape ==# ''
    ShowList(root)
    return
  endif
  server.Start(root, (_, err) => {
    if err != v:null
      Warn(err.message)
      return
    endif
    server.Request('tape/open', {tapeId: tape, withText: true}, (res, err2) => {
      if err2 != v:null
        Warn(err2.code ==# 'tape_not_found' ? 'テープが見つからない: ' .. tape : err2.message)
        return
      endif
      live.Close()
      list.Close()
      replay.Open(tape, res.frames, root)
    })
  })
enddef

export def Live()
  const root = getcwd()
  server.SetHandlers(live.OnNotify, OnServerExit)
  server.Start(root, (_, err) => {
    if err != v:null
      Warn(err.message)
      return
    endif
    server.Request('live/start', {withText: true}, (res, err2) => {
      if err2 != v:null
        Warn(err2.message)
        return
      endif
      replay.Close()
      list.Close()
      live.Start(root, res.tapeId, res.frames)
    })
  })
enddef

def OnServerExit(_why: string)
  if live.Active()
    live.Close()
    Warn('表示サーバーが終了したので、ライブ視聴を閉じた')
  endif
enddef

export def Close()
  replay.Close()
  live.Close()
enddef

export def Next()
  replay.StepForward()
enddef

export def Prev()
  replay.StepBack()
enddef

# Latest goes back to the newest frame of the live view and follows again.
export def Latest()
  replay.Latest()
enddef

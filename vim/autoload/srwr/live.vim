vim9script

import autoload './replay.vim'
import autoload './server.vim'

# Live is the replay screen on a list that grows. This file only starts and stops watching and hands the
# server's notifications to the replay; following the newest frame is the replay's job (the same Goto).

def Ignore(_result: any, _err: any)
enddef

def StopWatching()
  if server.Running()
    server.Request('live/stop', {}, Ignore)
  endif
enddef

export def Active(): bool
  return replay.Active() && replay.Session().live
enddef

export def Session(): dict<any>
  return Active() ? replay.Session() : {}
enddef

# Start opens the live tab. tapeId and frames come from live/start (the past frames are only listed).
export def Start(root: string, tapeId: any, frames: list<dict<any>>)
  replay.SetOnLiveClose(StopWatching)
  replay.OpenLive(tapeId, frames, root)
enddef

# OnNotify is the handler of the server's notifications.
export def OnNotify(method: string, params: dict<any>)
  if method ==# 'live/frame' && Active()
    replay.Append(params.tapeId, params.frame)
  endif
enddef

export def Close()
  if Active()
    replay.Close()
  endif
enddef

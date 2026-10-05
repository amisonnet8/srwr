vim9script

import autoload './lang.vim'
import autoload './list.vim'
import autoload './live.vim'
import autoload './replay.vim'
import autoload './server.vim'
import autoload './timeline.vim'

# The entry of every command.

def Warn(msg: string)
  echohl ErrorMsg
  echomsg 'srwr: ' .. msg
  echohl None
enddef

# The kinds of frames that are shown. Not saved: a new start of Vim is the default (srwr has no settings for the look).
const ALL_KINDS = ['look', 'edit', 'external', 'failure']
var kinds: list<string> = ['look', 'edit', 'external']

export def Kinds(): list<string>
  return copy(kinds)
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

# OpenTape opens a tape with the kinds that are shown, at the frame nearest to seq (the first frame when seq < 0).
def OpenTape(tape: string, root: string, seq: number)
  server.Request('tape/open', {tapeId: tape, withText: true, kinds: kinds}, (res, err2) => {
    if err2 != v:null
      Warn(err2.code ==# 'tape_not_found' ? lang.Pick('Tape not found: ', 'テープが見つからない: ') .. tape : err2.message)
      return
    endif
    live.Close()
    list.Close()
    const at = seq < 0 ? 0 : max([timeline.NearestBySeq(timeline.New(res.frames), seq), 0])
    replay.Open(tape, res.frames, root, false, get(res, 'hidden', {}), at)
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
    OpenTape(tape, root, -1)
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
    server.Request('live/start', {withText: true, kinds: kinds}, (res, err2) => {
      if err2 != v:null
        Warn(err2.message)
        return
      endif
      replay.Close()
      list.Close()
      live.Start(root, res.tapeId, res.frames, get(res, 'hidden', {}))
    })
  })
enddef

# Toggle turns a kind of frame on or off (ts, tr, te, tf; :SrwrToggle). An open tape is opened again with the new kinds, at the frame
# nearest to the one on the screen; the live view starts again and follows the newest.
export def Toggle(kind: string)
  if index(ALL_KINDS, kind) < 0
    Warn(lang.Pick('Unknown kind: ', '知らない種類: ') .. kind .. ' (' .. join(ALL_KINDS, ', ') .. ')')
    return
  endif
  const i = index(kinds, kind)
  if i >= 0
    remove(kinds, i)
  else
    kinds = filter(copy(ALL_KINDS), (_, k) => index(kinds, k) >= 0 || k ==# kind)
  endif
  echomsg 'srwr: ' .. lang.Pick('showing ', '表示: ') .. (empty(kinds) ? lang.Pick('nothing', 'なし') : join(kinds, ', '))
  if live.Active()
    Live()
  elseif replay.Active()
    const sess = replay.Session()
    const f = get(sess.tl.frames, sess.index, {})
    OpenTape(sess.tape, sess.root, get(f, 'seq', -1))
  endif
enddef

def OnServerExit(_why: string)
  if live.Active()
    live.Close()
    Warn(lang.Pick('The view server exited, so the live view was closed', '表示サーバーが終了したので、ライブ視聴を閉じた'))
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

replay.SetOnToggle(Toggle)

# Latest goes back to the newest frame of the live view and follows again.
export def Latest()
  replay.Latest()
enddef

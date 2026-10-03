vim9script
# What a person sees when something goes wrong.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/list.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/server.vim'
import autoload 'srwr/ui.vim'

def Srwr(): list<string>
  return filter(mapnew(getbufinfo(), (_, b) => b.name), (_, n) => n =~# '^srwr://')
enddef

def Messages(): string
  return execute('messages')
enddef

def EndServer()
  server.Stop()
  t.WaitFor((): bool => !server.Running(), 'the server to end')
enddef

# A stand-in for srwr: a script that answers the first request the way the test wants.
def StandIn(body: string): string
  const path = tempname()
  writefile(['#!/bin/sh', body], path)
  setfperm(path, 'rwx------')
  return path
enddef

t.Workspace()

# --- the binary is not there ---
g:srwr_path = '/no/such/srwr'
messages clear
ui.Open('20260930-0054-why-basic')
t.True(Messages() =~# "srwr: srwr のバイナリが見つからない（g:srwr_path = '/no/such/srwr'）。srwr を入れるか、g:srwr_path に場所を指定する", 'binary not found (open)')
t.Equal(1, tabpagenr('$'), 'no tab was opened')
messages clear
ui.Live()
t.True(Messages() =~# 'srwr のバイナリが見つからない', 'binary not found (live)')
messages clear
ui.Open('')
t.True(Messages() =~# 'srwr のバイナリが見つからない', 'binary not found (list)')
t.Equal([], Srwr(), 'nothing was left')

# --- the version does not match ---
g:srwr_path = StandIn("read l; echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32000,\"message\":\"x\",\"data\":{\"code\":\"protocol_mismatch\"}}}'; sleep 2")
messages clear
ui.Open('x')
t.WaitFor((): bool => Messages() =~# 'バージョンが合っていません', 'the mismatch notice')
t.True(Messages() =~# 'protocolVersion 1', 'it names the protocol version')
EndServer()

# --- the server ends while a request waits ---
g:srwr_path = StandIn("echo 'boom' >&2; read l; exit 3")
messages clear
ui.Open('x')
t.WaitFor((): bool => Messages() =~# 'srwr view-server が終了した（終了コード 3）', 'the exit notice')
t.True(Messages() =~# 'boom', 'with the end of its error output')
t.Equal([], Srwr(), 'nothing was left')
EndServer()

# --- the tape is not there ---
g:srwr_path = t.Root() .. '/bin/srwr'
messages clear
ui.Open('20200101-0000-none')
t.WaitFor((): bool => Messages() =~# 'srwr: テープが見つからない: 20200101-0000-none', 'tape not found')
t.Equal(1, tabpagenr('$'), 'no tab was opened')

# --- a tape id that is not an id is refused by the server ---
messages clear
ui.Open('..')
t.WaitFor((): bool => Messages() =~# 'srwr: ', 'an invalid tape id')
t.Equal(1, tabpagenr('$'), 'no tab was opened for it')

# --- a path to a tape file is accepted ---
ui.Open('.srwr/tapes/20260930-0054-why-basic.tape.jsonl')
t.WaitFor((): bool => replay.Active(), 'a tape given by its path')
t.Equal('20260930-0054-why-basic', replay.Session().tape, 'the id of that path')
ui.Close()

t.Finish()

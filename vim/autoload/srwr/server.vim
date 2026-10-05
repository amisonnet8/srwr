vim9script

import autoload './config.vim'
import autoload './lang.vim'

# The only place that talks to the display server (docs/reference/protocol.md):
# `srwr view-server` as a job, newline-delimited JSON-RPC 2.0 on a channel in "nl" mode.
# Vim has no promises, so a request takes a callback: Cb(result, err). One of them is v:null.
# An err is {code: srwr error code (or 'server_exited' ...), message: string}.

export const PROTOCOL_VERSION = 2

var job: any = v:null
var nextId = 0
var pending: dict<func(any, any)> = {}

def IgnoreNotify(_method: string, _params: dict<any>)
enddef

def IgnoreExit(_why: string)
enddef

var OnNotify: func(string, dict<any>) = IgnoreNotify
var OnExit: func(string) = IgnoreExit
var stderrLines: list<string> = []

def Fail(code: string, message: string): dict<any>
  return {code: code, message: message}
enddef

def HandleLine(_ch: channel, line: string)
  var msg: any
  try
    msg = json_decode(line)
  catch
    return
  endtry
  if type(msg) != v:t_dict
    return
  endif
  if has_key(msg, 'id') && has_key(pending, string(msg.id))
    const key = string(msg.id)
    const Cb = pending[key]
    remove(pending, key)
    if has_key(msg, 'error') && type(msg.error) == v:t_dict
      const e: dict<any> = msg.error
      const data: any = get(e, 'data', {})
      const code = type(data) == v:t_dict && has_key(data, 'code') ? data.code : 'rpc_error'
      Cb(v:null, Fail(code, get(e, 'message', '')))
    else
      Cb(get(msg, 'result', v:null), v:null)
    endif
  elseif has_key(msg, 'method')
    OnNotify(msg.method, get(msg, 'params', {}))
  endif
enddef

def HandleExit(_job: job, status: number)
  job = v:null
  const waiting = pending
  pending = {}
  for Cb in values(waiting)
    Cb(v:null, Fail('server_exited', lang.Pick('srwr view-server exited (exit code ' .. status .. '). ', 'srwr view-server が終了した（終了コード ' .. status .. '）。') .. join(stderrLines[-3 :], ' ')))
  endfor
  OnExit('exited')
enddef

# Running tells whether the server process is up.
export def Running(): bool
  return job != v:null && job_status(job) ==# 'run'
enddef

# Pid is the process id of the server, or 0 when it is not running (the tests end the server with it).
export def Pid(): number
  return Running() ? job_info(job).process : 0
enddef

# SetHandlers registers what to do with a notification (live/frame) and with the server going away.
export def SetHandlers(Notify: func(string, dict<any>), Exit: func(string))
  OnNotify = Notify
  OnExit = Exit
enddef

# Start launches `srwr view-server --root <root>` and sends initialize. Done(result, err) is called once.
export def Start(root: string, Done: func(any, any))
  if Running()
    Initialize(Done)
    return
  endif
  const cmd = config.Path()
  if !executable(cmd)
    Done(v:null, Fail('binary_not_found', lang.Pick('srwr binary not found (g:srwr_path = ' .. string(cmd) .. '). Install srwr or set g:srwr_path', 'srwr のバイナリが見つからない（g:srwr_path = ' .. string(cmd) .. '）。srwr を入れるか、g:srwr_path に場所を指定する')))
    return
  endif
  stderrLines = []
  job = job_start([cmd, 'view-server', '--root', root], {
    mode: 'nl',
    out_cb: HandleLine,
    err_mode: 'nl',
    err_cb: (_, l) => {
      add(stderrLines, l)
    },
    exit_cb: HandleExit,
    stoponexit: 'term',
  })
  if job_status(job) ==# 'fail'
    job = v:null
    Done(v:null, Fail('binary_not_found', lang.Pick('Cannot start srwr view-server: ', 'srwr view-server を起動できない: ') .. cmd))
    return
  endif
  Initialize(Done)
enddef

# Initialize sends initialize to the running server.
def Initialize(Done: func(any, any))
  Request('initialize', {client: 'vim', protocolVersion: PROTOCOL_VERSION, options: config.ServerOptions()}, (res, err) => Done(res, FixMismatch(err)))
enddef

# FixMismatch gives protocol_mismatch a message a person can act on.
def FixMismatch(err: any): any
  if err != v:null && err.code ==# 'protocol_mismatch'
    return Fail('protocol_mismatch', lang.Pick('srwr (' .. config.Path() .. ') and this Vim script do not match (protocolVersion ' .. PROTOCOL_VERSION .. '). Update srwr.', 'srwr（' .. config.Path() .. '）と、この Vim スクリプトのバージョンが合っていません（protocolVersion ' .. PROTOCOL_VERSION .. '）。srwr を更新してください。'))
  endif
  return err
enddef

# Request sends a request. Cb is called with the result, or with an error.
export def Request(method: string, params: dict<any>, Cb: func(any, any))
  if !Running()
    Cb(v:null, Fail('server_exited', lang.Pick('srwr view-server is not running', 'srwr view-server は動いていない')))
    return
  endif
  nextId += 1
  pending[string(nextId)] = Cb
  ch_sendraw(job, json_encode({jsonrpc: '2.0', id: nextId, method: method, params: params}) .. "\n")
enddef

# Stop asks the server to end, then makes sure the process is gone.
export def Stop()
  if job == v:null
    return
  endif
  const j = job
  if job_status(j) ==# 'run'
    ch_sendraw(j, json_encode({jsonrpc: '2.0', id: 0, method: 'shutdown', params: {}}) .. "\n")
    timer_start(500, (_) => job_status(j) ==# 'run' ? job_stop(j) : 0)
  endif
enddef

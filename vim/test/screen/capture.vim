vim9script
# Takes the screen of a real Vim, frame by frame (docs: .claude/rules/vim.md, "画面なしの Vim では…").
#
# This runs in an outer Vim that has a terminal (vim/screen_test.go starts it with `script`). The outer Vim starts an inner
# Vim with srwr-view.vim in a terminal window of 140x50 and reads its screen with term_scrape(). Every wait is for a
# condition on the screen, never for a time.
#
# Environment: SRWR_REPO (the repository), SRWR_BIN (the srwr binary), WS (a copy of the fixed workspace),
# SCENARIO (replay:<tape> | live-basic | live-external), THEME (dark | light), OUT (the file to write),
# ROWS and COLS (the size of the screen; 50 and 140 by default).
set nocompatible

const repo = $SRWR_REPO
const theme = $THEME
const scenario = $SCENARIO
const ROWS = empty($ROWS) ? 50 : str2nr($ROWS)
const COLS = empty($COLS) ? 140 : str2nr($COLS)

# The inner Vim: the colors of the images (docs/design: Normal is set, everything else is Vim's own default).
const vimrc = tempname()
writefile([
  'set nocompatible',
  'set termguicolors',
  'set background=' .. theme,
  'syntax on',
  'filetype plugin on',
  theme ==# 'dark' ? 'highlight Normal guifg=#d4d4d4 guibg=#1e1e1e' : 'highlight Normal guifg=#1f2328 guibg=#ffffff',
], vimrc)

def Start(command: string): number
  const cmd = [empty($SRWR_VIM_BIN) ? 'vim' : $SRWR_VIM_BIN, '-N', '-u', vimrc, '-i', 'NONE',
    '--cmd', 'let &runtimepath = "' .. repo .. '/vim," . &runtimepath',
    '--cmd', 'let g:srwr_path = "' .. $SRWR_BIN .. '"',
    '-c', command]
  return term_start(cmd, {term_rows: ROWS, term_cols: COLS, cwd: $WS, env: {COLORTERM: 'truecolor', TERM: 'xterm-256color'}, curwin: true})
enddef

var term = 0

def Rows(): list<list<dict<any>>>
  return mapnew(range(1, ROWS), (_, r) => term_scrape(term, r))
enddef

def Plain(rows: list<list<dict<any>>>): string
  return join(mapnew(rows, (_, row) => join(mapnew(row, (_, c) => c.chars), '')), "\n")
enddef

# Wait until the screen matches re and has stopped changing.
def WaitScreen(re: string, what: string)
  var waited = 0
  var last = ''
  while true
    const now = Plain(Rows())
    if now =~# re && now ==# last
      return
    endif
    last = now
    if waited > 15000
      writefile(['timed out waiting for ' .. what, now], $OUT .. '.error')
      cquit 1
    endif
    sleep 40m
    waited += 40
  endwhile
enddef

var frames: list<dict<any>> = []

# One cell: [text, foreground, background, bold, width]. A reversed cell is given with its colors swapped, as it looks.
def Cell(c: dict<any>): list<any>
  const rev = term_getattr(c.attr, 'reverse') != 0
  return [c.chars, rev ? c.bg : c.fg, rev ? c.fg : c.bg, term_getattr(c.attr, 'bold') != 0, c.width]
enddef

def Record(label: string)
  add(frames, {label: label, rows: mapnew(Rows(), (_, row) => mapnew(row, (_, c) => Cell(c)))})
enddef

def Keys(keys: string)
  term_sendkeys(term, keys)
enddef

def Position(i: number, n: number): string
  return 'srwr  ' .. i .. '/' .. n .. ' '
enddef

# --- scenarios ---

def Replay(tape: string)
  term = Start('SrwrOpen ' .. tape)
  WaitScreen('srwr  1/\d\+', 'the first frame')
  const n = str2nr(matchlist(Plain(Rows()), 'srwr  1/\(\d\+\)')[1])
  for i in range(1, n)
    if i > 1
      Keys(']]')
    endif
    WaitScreen('srwr  ' .. i .. '/' .. n .. ' ', 'frame ' .. i)
    Record(string(i))
  endfor
enddef

def TapeLines(name: string): list<string>
  return readfile(repo .. '/extension/test/fixtures/ui-check/.srwr/tapes/' .. name .. '.tape.jsonl')
enddef

def IsFrame(line: string): bool
  return index(['select', 'replace', 'external'], json_decode(line).type) >= 0
enddef

var written = 0
var counted = 0
var all: list<string> = []
var liveTape = ''

# Write lines into the live tape until it holds n frames.
def Append(n: number)
  while counted < n
    writefile([all[written]], liveTape, 'a')
    counted += IsFrame(all[written]) ? 1 : 0
    written += 1
  endwhile
enddef

def Live(source: string, first: number, stages: list<list<any>>)
  all = TapeLines(source)
  liveTape = $WS .. '/.srwr/tapes/20260930-0000-live.tape.jsonl'
  writefile([], liveTape)
  Append(first)
  term = Start('SrwrLive')
  WaitScreen('srwr  1/' .. first .. ' ', 'the live view')
  for st in stages
    # st = [label, action, count, position, total]
    const [label, action, count, pos, total] = [st[0], st[1], st[2], st[3], st[4]]
    if action ==# 'append'
      Append(count)
    elseif action ==# 'back'
      Keys('[[')
    elseif action ==# 'latest'
      Keys('L')
    endif
    WaitScreen(Position(pos, total), label)
    Record(label)
  endfor
enddef

if scenario =~# '^replay:'
  Replay(scenario[7 :])
elseif scenario ==# 'live-basic'
  Live('20260930-0054-why-basic', 1, [
    ['L1_waiting', 'none', 0, 1, 1],
    ['L2_following_1', 'append', 2, 2, 2],
    ['L3_following_3', 'append', 3, 3, 3],
    ['L4_stepped_back', 'back', 0, 2, 3],
    ['L5_behind_2', 'append', 5, 2, 5],
    ['L6_behind_4', 'append', 7, 2, 7],
    ['L7_back_to_live', 'latest', 0, 7, 7],
  ])
elseif scenario ==# 'live-external'
  Live('20260930-0949-external', 1, [
    ['E1_following', 'append', 5, 5, 5],
    ['E2_external', 'append', 6, 6, 6],
    ['E3_after_external', 'append', 7, 7, 7],
  ])
endif

writefile([json_encode({cols: COLS, rows: ROWS, frames: frames})], $OUT)
qall!

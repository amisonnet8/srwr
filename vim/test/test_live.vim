vim9script
# The live view against the real display server: a tape in a workspace grows while the view is open.
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/live.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/server.vim'
import autoload 'srwr/ui.vim'

def Lines(name: string): list<string>
  return readfile(t.Root() .. '/extension/test/fixtures/ui-check/.srwr/tapes/' .. name .. '.tape.jsonl')
enddef

def IsFrame(line: string): bool
  return index(['select', 'replace', 'look', 'edit', 'external'], json_decode(line).type) >= 0
enddef

# The lines up to and including the n-th frame.
def Upto(all: list<string>, n: number): number
  var count = 0
  for i in range(len(all))
    if IsFrame(all[i])
      count += 1
      if count == n
        return i
      endif
    endif
  endfor
  return len(all) - 1
enddef

def Srwr(): list<string>
  return filter(mapnew(getbufinfo(), (_, b) => b.name), (_, n) => n =~# '^srwr://')
enddef

def Frames(): number
  return len(replay.Session().tl.frames)
enddef

def StatusIs(re: string): bool
  return getwinvar(replay.Session().win, '&statusline') =~# re
enddef

set columns=140 lines=50
const dir = t.Workspace(false)
const tape = dir .. '/.srwr/tapes/20260930-0000-live.tape.jsonl'
const lines = Lines('20260930-0054-why-basic')
var written = Upto(lines, 1) + 1
writefile(lines[0 : written - 1], tape)

def Append(n: number)
  # Appends lines until the tape holds n frames.
  var count = 0
  for l in lines[0 : written - 1]
    count += IsFrame(l) ? 1 : 0
  endfor
  while count < n
    writefile([lines[written]], tape, 'a')
    count += IsFrame(lines[written]) ? 1 : 0
    written += 1
  endwhile
enddef

ui.Live()
t.WaitFor((): bool => live.Active(), 'the live view to open')
var s = live.Session()

# --- start: the frames so far are listed, nothing is shown ---
t.Equal(1, Frames(), 'the frame so far is listed')
t.Equal(['ライブ視聴中（AI の操作を待っています）'], getbufline(s.buf, 1, '$'), 'the waiting text')
t.Equal(1, line('$', s.sidebar), 'one row in the list')
t.True(StatusIs('● LIVE'), 'following')
t.Equal(0, s.index, 'the position is the last frame')

# --- following: a new frame is shown at once ---
Append(2)
t.WaitFor((): bool => Frames() == 2, 'the second frame')
t.Equal(1, s.index, 'the new frame is shown')
t.Equal('srwr://20260930-0000-live/text.go', bufname(s.buf), 'the file of the frame')
t.True(getbufline(s.buf, 1, '$')[102] =~# '^◆ Wrap', 'the frame is on the screen: its why row is in front of line 103')
t.True(StatusIs('srwr  2/2  .*● LIVE'), 'status while following')

# a burst
Append(5)
t.WaitFor((): bool => Frames() == 5, 'the frames 3 to 5')
t.Equal(4, s.index, 'a burst is followed to the last frame')
t.Equal(true, StatusIs('srwr  5/5'), 'position')

# --- stepping back stops following; new frames are only counted ---
replay.StepBack()
t.Equal(3, s.index, 'one frame back')
t.True(StatusIs('%#SrwrWhyReplace# L：LIVE に戻る（新着 1） %\*'), 'the orange live mark with 1')
const shown = getbufline(s.buf, 1, '$')
Append(7)
t.WaitFor((): bool => Frames() == 7, 'the frames 6 and 7')
t.Equal(3, s.index, 'the screen did not move')
t.Equal(shown, getbufline(s.buf, 1, '$'), 'and the text did not change')
t.True(StatusIs('L：LIVE に戻る（新着 3）'), 'the mark counts the new frames')
t.Equal(7, line('$', s.sidebar), 'but the list grew')

# the same Goto: stepping forward to the end follows again
replay.Latest()
t.Equal(6, s.index, 'back at the latest')
t.True(StatusIs('● LIVE'), 'following again')
t.Equal(false, StatusIs('LIVE に戻る'), 'the orange mark is gone')

# --- another tape takes over: the list starts again from its first frame ---
const other = Lines('20260930-0949-external')
const otherTape = dir .. '/.srwr/tapes/20260930-0001-other.tape.jsonl'
sleep 1100m
writefile(other[0 : Upto(other, 1)], otherTape)
t.WaitFor((): bool => Frames() == 1 && bufname(s.buf) =~# '20260930-0001-other', 'the new tape to take over')
t.Equal(1, line('$', s.sidebar), 'one row for the new tape')
t.Equal('srwr://20260930-0001-other/entry.go', bufname(s.buf), 'its first frame is shown')
t.True(StatusIs('srwr  1/1  .*● LIVE'), 'following the new tape')

# --- closing ---
ui.Close()
t.Equal(false, live.Active(), 'closed')
t.Equal([], Srwr(), 'no buffer is left')
t.Equal(1, tabpagenr('$'), 'no tab is left')

# --- the server goes away: the live view closes and says so ---
ui.Live()
t.WaitFor((): bool => live.Active(), 'the live view to open again')
t.True(server.Pid() > 0, 'the server is running')
system('kill ' .. server.Pid())
t.WaitFor((): bool => !live.Active(), 'the live view to close when the server ends')
t.True(execute('messages') =~# 'ライブ視聴を閉じた', 'the notice')
t.Equal([], Srwr(), 'no buffer is left after the server ended')

t.Finish()

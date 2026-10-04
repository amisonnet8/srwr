vim9script
# The two problems carried over from the previous implementation, as the UI gate of R5 decided:
#  1. a range near the bottom of the window was hidden under the why rows (RevealTop)
#  2. the live mark "L：LIVE に戻る（新着 N）" was cut off by the close hint (StatusParts)
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/replay.vim'

# --- 1. RevealTop(topline, height, first, last, total): where to scroll so first..last are all visible ---
t.Equal(-1, replay.RevealTop(1, 37, 5, 6, 100), 'already visible: no scroll')
t.Equal(-1, replay.RevealTop(1, 37, 36, 37, 100), 'the block ends on the last row: visible')
t.Equal(19, replay.RevealTop(1, 37, 37, 38, 100), 'the why row on the last row and the range below it: the why row in the middle (the carried-over bug)')
t.Equal(20, replay.RevealTop(1, 37, 38, 39, 100), 'the block fully below: the why row in the middle')
t.Equal(14, replay.RevealTop(1, 37, 48, 49, 50), 'near the end of the file: the last page, not an empty gap')
t.Equal(71, replay.RevealTop(1, 47, 103, 104, 117), 'the approved image of text.go:103: the last page of a 117-line file')
t.Equal(58, replay.RevealTop(1, 47, 81, 90, 128), 'the approved image of stats.go:81-89: the why row in the middle')
t.Equal(1, replay.RevealTop(30, 37, 2, 3, 100), 'above the window: centered, but not above line 1')
t.Equal(10, replay.RevealTop(1, 20, 10, 100, 200), 'a block taller than the window: its first line at the top')
t.Equal(-1, replay.RevealTop(1, 20, 1, 20, 200), 'a block exactly as tall as the window, in place')
t.Equal(11, replay.RevealTop(5, 20, 11, 30, 200), 'a block exactly as tall as the window, not in place: first line at the top')
t.Equal(-1, replay.RevealTop(1, 0, 5, 6, 10), 'no height: nothing to do')
t.Equal(1, replay.RevealTop(10, 10, 1, 1, 5), 'a short file')

# --- 2. StatusParts(index, total, live, where, room, lead) ---
def Plain(parts: list<list<string>>): string
  return join(mapnew(parts, (_, p) => p[0]), '')
enddef
def Styles(parts: list<list<string>>): list<string>
  return mapnew(parts, (_, p) => p[1])
enddef
const hint = '（閉じる：左の一覧で q、または :SrwrClose）'

t.Equal('srwr  2/3  [[ 戻る  ]] 進む  text.go:103', Plain(replay.StatusParts(1, 3, false, 'text.go:103', 100)), 'replay status')
t.Equal(['dim'], filter(Styles(replay.StatusParts(0, 3, false, '', 100)), (_, v) => v ==# 'dim'), 'first frame: back is dim')
t.Equal(['dim'], filter(Styles(replay.StatusParts(2, 3, false, '', 100)), (_, v) => v ==# 'dim'), 'last frame: forward is dim')
t.Equal([], filter(Styles(replay.StatusParts(1, 3, false, '', 100)), (_, v) => v ==# 'dim'), 'middle: nothing is dim')
t.Equal('srwr  ● LIVE  （AI の操作を待っています）', Plain(replay.StatusParts(-1, 0, true, '', 100)), 'live, nothing yet')
t.Equal('srwr  1/1  [[ 戻る  ]] 進む  ● LIVE  text.go:37  ' .. hint, Plain(replay.StatusParts(0, 1, true, 'text.go:37', 200)), 'following, wide window: with the hint')
t.Equal(false, Plain(replay.StatusParts(0, 1, true, 'text.go:37', 200)) !~# '● LIVE', 'following shows the mark')

# behind: the orange part is there, and the line fits when the hint cannot
const narrow = replay.StatusParts(1, 3, true, 'text.go:103', 99)
t.Equal(false, Plain(narrow) =~# hint, 'window of 99 cells: no hint')
t.Equal(true, Plain(narrow) =~# ' L：LIVE に戻る（新着 1） ', 'window of 99 cells: the live mark is whole')
t.Equal(['new'], filter(Styles(narrow), (_, v) => v ==# 'new'), 'the live mark is the orange part')
t.True(strdisplaywidth(Plain(narrow)) <= 99, 'the line fits in 99 cells')
t.Equal(true, Plain(replay.StatusParts(1, 3, true, 'text.go:103', 300)) =~# hint, 'wide window: the hint is back')

# the boundary: the hint is shown exactly when everything fits
const full = Plain(replay.StatusParts(1, 3, true, 'text.go:103', 1000))
const w = strdisplaywidth(full)
t.Equal(full, Plain(replay.StatusParts(1, 3, true, 'text.go:103', w)), 'room == width: the hint stays')
t.Equal(true, Plain(replay.StatusParts(1, 3, true, 'text.go:103', w - 1)) !~# hint, 'room == width - 1: the hint goes')
# in the right-hand window of a diff frame, what comes before counts
t.Equal(true, Plain(replay.StatusParts(1, 3, true, 'text.go:103', w, '後  ')) !~# hint, 'the lead takes room too')
t.Equal(full, Plain(replay.StatusParts(1, 3, true, 'text.go:103', w + 4, '後  ')), 'room == width + lead: the hint stays')
# a replay (not live) never shows the hint
t.Equal(true, Plain(replay.StatusParts(1, 3, false, 'text.go:103', 1000)) !~# hint, 'no hint outside live')

# the value for 'statusline'
t.Equal('srwr  1/3  %#SrwrDim#[[ 戻る%*  ]] 進む%<  a%%b.go:1', replay.StatusString(replay.StatusParts(0, 3, false, 'a%b.go:1', 100)), 'statusline value: dim part and escaped percent')
t.Equal(true, replay.StatusString(replay.StatusParts(1, 3, true, '', 80)) =~# '%#SrwrWhyReplace# L：LIVE に戻る（新着 1） %\*', 'statusline value: orange part')
t.Equal('50%%  後', replay.StatusString([['後', '']], '50%  '), 'the lead is escaped')

t.Finish()

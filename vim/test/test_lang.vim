vim9script
# English is the default; SRWR_LANG=ja gives the Japanese texts (the other tests look at those). The time of a tape is shown in
# the time zone of the machine ($TZ).
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/diff.vim'
import autoload 'srwr/lang.vim'
import autoload 'srwr/list.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/sidebar.vim'
import autoload 'srwr/timeline.vim'

def Plain(parts: list<list<string>>): string
  return join(mapnew(parts, (_, p) => p[0]), '')
enddef

# --- which language ---
$SRWR_LANG = ''
t.Equal(false, lang.Ja(), 'nothing set: English')
$SRWR_LANG = 'ja_JP.UTF-8'
t.Equal(true, lang.Ja(), 'ja_JP: Japanese')
$SRWR_LANG = 'en'
t.Equal(false, lang.Ja(), 'en: English')
$SRWR_LANG = 'xja'
t.Equal(false, lang.Ja(), 'a name that only contains ja: English')

# --- English ---
$SRWR_LANG = ''
t.Equal('srwr  2/3  [[ Back  ]] Forward  text.go:103', Plain(replay.StatusParts(1, 3, false, 'text.go:103', 1000)), 'status line')
t.Equal("srwr  ● LIVE  (waiting for the AI's operations)", Plain(replay.StatusParts(-1, 0, true, '', 1000)), 'live, nothing yet')
t.Equal('srwr  2/5  [[ Back  ]] Forward   L: Back to LIVE (3 new)   text.go:1  (Close: q in the list on the left, or :SrwrClose)', Plain(replay.StatusParts(1, 5, true, 'text.go:1', 1000)), 'live, behind, wide window')
t.True(Plain(replay.StatusParts(1, 5, true, 'text.go:1', 80)) !~# 'Close:', 'the hint goes first when the window is narrow')
t.Equal('before 12', timeline.FormatRange({start: 12, end: 11}), 'empty range')
t.Equal('⚠ Changed outside srwr: stats.go', diff.Label({kind: 'external', file: 'src/stats.go'}), 'external heading')
t.Equal('⚠ Changed outside srwr (deleted): stats.go', diff.Label({kind: 'external', file: 'stats.go', deleted: true}), 'deleted heading')
t.Equal('⚠ Changed after recording (diff from current file): stats.go', diff.Label({kind: 'final', file: 'stats.go'}), 'final heading')
t.Equal('⚠ Changed after recording (no longer exists): stats.go', diff.Label({kind: 'final', file: 'stats.go', deleted: true}), 'final, gone')
t.Equal('Started           Updated            Ops     Files  Tape', list.Header(), 'heading of the list')
t.Equal('●  3 external stats.go:1-3', sidebar.Line({index: 2, kind: 'external', file: 'stats.go', range: {start: 1, end: 3}, why: v:null}), 'a kind in the list')
t.Equal('●  1 look     a.go:1', sidebar.Line({index: 0, kind: 'look', file: 'a.go', range: {start: 1, end: 1}, why: v:null}), 'look is as wide as external')

# --- Japanese ---
$SRWR_LANG = 'ja'
t.Equal('srwr  2/3  [[ 戻る  ]] 進む  text.go:103', Plain(replay.StatusParts(1, 3, false, 'text.go:103', 1000)), 'status line in Japanese')
t.Equal('12の前', timeline.FormatRange({start: 12, end: 11}), 'empty range in Japanese')

# --- time: the tape holds UTC (or an offset), the list shows the zone of the machine ---
$SRWR_LANG = ''
$TZ = 'Asia/Tokyo'
t.Equal('2026-10-03 17:12', list.Time('2026-10-03T08:12:10.000Z'), 'UTC shown in Tokyo')
t.Equal('2026-10-03 17:12', list.Time('2026-10-03T17:12:10.000+09:00'), 'an older tape (+09:00) shown in Tokyo')
$TZ = 'UTC'
t.Equal('2026-10-03 08:12', list.Time('2026-10-03T08:12:10.000Z'), 'UTC shown in UTC')
t.Equal('2026-10-03 08:12', list.Time('2026-10-03T17:12:10.000+09:00'), 'an older tape (+09:00) shown in UTC')
t.Equal('2026-10-03 18:12', list.Time('2026-10-03T08:12:10-10:00'), 'a negative offset (08:12 at -10:00 is 18:12 UTC)')
t.Equal('2026-03-01 00:00', list.Time('2026-02-28T23:30:00-00:30'), 'a day that rolls over (and a month, in a year that is not a leap year)')
t.Equal('2024-02-29 23:59', list.Time('2024-02-29T23:59:00Z'), 'a leap day')
t.Equal('2024-03-01 00:00', list.Time('2024-02-29T23:59:00-00:01'), 'a leap day rolls over to March')
t.Equal('', list.Time(v:null), 'not a string')
t.Equal('not a time', list.Time('not a time'), 'text that is not a time is shown as it is')

t.Finish()

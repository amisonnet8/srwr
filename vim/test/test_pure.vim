vim9script
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/buf.vim'
import autoload 'srwr/list.vim'
import autoload 'srwr/paint.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/sidebar.vim'
import autoload 'srwr/timeline.vim'
import autoload 'srwr/ui.vim'
import autoload 'srwr/config.vim'

# --- timeline ---
const tl = timeline.New([
  {index: 0, kind: 'look', file: 'a.go', before: 'a1', after: 'a1', range: {start: 1, end: 1}, why: 'w'},
  {index: 1, kind: 'edit', file: 'b.go', before: 'b1', after: 'b2', range: {start: 1, end: 1}, why: v:null},
  {index: 2, kind: 'edit', file: 'a.go', before: 'a1', after: 'a3', range: {start: 1, end: 2}, why: 'w'},
])
t.Equal('a1', timeline.ContentAt(tl, 'a.go', 0), 'content after the first frame')
t.Equal('a1', timeline.ContentAt(tl, 'a.go', 1), 'the file is as the last frame that touched it left it')
t.Equal('a3', timeline.ContentAt(tl, 'a.go', 2), 'content after the replace')
t.Equal('b1', timeline.ContentAt(tl, 'b.go', 0), 'a file nobody touched yet is as before the first frame that touches it')
t.Equal('b2', timeline.ContentAt(tl, 'b.go', 2), 'content of b.go at the end')
t.Equal(v:null, timeline.ContentAt(tl, 'c.go', 1), 'no frame touches the file')
t.Equal('a1', timeline.ContentAt(tl, 'a.go', -1), 'before any frame')
t.Equal('', timeline.Why(tl.frames[1]), 'a null why is empty')
t.Equal('select', timeline.Tone({kind: 'look'}), 'tone of a look')
for k in ['edit', 'new', 'external', 'final']
  t.Equal('replace', timeline.Tone({kind: k}), 'tone ' .. k)
endfor
t.True(timeline.IsDiff({kind: 'external'}) && timeline.IsDiff({kind: 'final'}) && !timeline.IsDiff({kind: 'edit'}), 'diff kinds')
t.Equal('37', timeline.FormatRange({start: 37, end: 37}), 'one line')
t.Equal('39-41', timeline.FormatRange({start: 39, end: 41}), 'many lines')
t.Equal('12の前', timeline.FormatRange({start: 12, end: 11}), 'empty range')
t.Equal('c.go', timeline.Basename('a/b/c.go'), 'basename')

# --- lines of a text ---
t.Equal([], buf.Lines(''), 'no lines')
t.Equal([], buf.Lines(v:null), 'null content')
t.Equal(['a', 'b'], buf.Lines("a\nb\n"), 'final newline')
t.Equal(['a', 'b'], buf.Lines("a\nb"), 'no final newline')
t.Equal(['a', ''], buf.Lines("a\n\n"), 'a blank last line')

# --- why rows ---
t.Equal(['◆ 理由です'], replay.Wrap('理由です', 100), 'one short row')
t.Equal(['◆ あああああ', '  あああああ'], replay.Wrap(repeat('あ', 10), 12), 'wide characters count 2, rows are indented')
t.Equal(['◆ abcdefgh', '  ij'], replay.Wrap('abcdefghij', 10), 'ASCII counts 1')
t.Equal(['◆ a', '  ', '  b'], replay.Wrap("a\n\nb", 100), 'a newline starts a row')
t.Equal(repeat('x', 40), join(map(replay.Wrap(repeat('x', 40), 1), (_, r) => substitute(r, '^[◆ ]* ', '', ''))[0 : ], ''), 'a very narrow width keeps everything')
t.Equal([], replay.BannerRows({why: v:null}, 80), 'no why, no rows')
t.Equal([], replay.BannerRows({why: ''}, 80), 'empty why, no rows')
t.Equal(1, len(replay.BannerRows({why: 'x'}, 80)), 'a why gives rows')
t.Equal(['a', 'W', 'b', 'c'], replay.WithBanner(['a', 'b', 'c'], 2, ['W']), 'rows go in front of the start line')
t.Equal(['W', 'a'], replay.WithBanner(['a'], 0, ['W']), 'a start before the content goes to the top')
t.Equal(['a', 'W'], replay.WithBanner(['a'], 99, ['W']), 'a start after the content goes to the end')
t.Equal(['W'], replay.WithBanner([], 1, ['W']), 'into nothing')
t.Equal(4, replay.BannerAt(4, 9), 'banner position')
t.Equal(10, replay.BannerAt(99, 9), 'banner position clamped to the end')
t.Equal(1, replay.BannerAt(-3, 9), 'banner position clamped to the top')

# --- the file's own line numbers beside the why rows ---
t.Equal(['  1 ', '  2 ', '    ', '    ', '  3 '], paint.NumberLabels(5, 3, 2), 'labels: why rows are blank, the rest keep the file numbers')
t.Equal(['  1 ', '    '], paint.NumberLabels(2, 2, 1), 'labels: why row at the end')
t.Equal(['    ', '  1 '], paint.NumberLabels(2, 1, 1), 'labels: why row at the top')
t.Equal(['    ', '    ', '  1 ', '  2 '], paint.NumberLabels(4, 1, 2), 'labels: two why rows at the top')
t.Equal(5, strchars(paint.NumberLabels(1200, 1, 2)[2]), 'the width grows with the number of file lines')

# --- the operation list ---
t.Equal('●  5 edit    text.go:37  幅ちょうどの', sidebar.Line({index: 4, kind: 'edit', file: 'src/text.go', range: {start: 37, end: 37}, why: '幅ちょうどの'}), 'row with why')
t.Equal('●  6 外部変更 stats.go:1-126  srwr の外でファイルが変わった', sidebar.Line({index: 5, kind: 'external', file: 'stats.go', range: {start: 1, end: 126}, why: v:null}), 'external without why says what it is')
t.Equal('●  1 edit    a.go:1', sidebar.Line({index: 0, kind: 'edit', file: 'a.go', range: {start: 1, end: 1}, why: v:null}), 'an edit without why')
t.Equal('srwr_dot_external', sidebar.DotType('final'), 'final is purple')
t.Equal('srwr_dot_select', sidebar.DotType('look'), 'look is blue')

# --- the tape list ---
t.Equal('2026-09-30 00:54', list.Time('2026-09-30T00:54:08.123+09:00'), 'time')
t.Equal('', list.Time(v:null), 'no time')
t.Equal('2026-09-30 00:54  2026-09-30 01:00     7         2  20260930-0054-why-basic',
  list.Line({startedAt: '2026-09-30T00:54:08+09:00', updatedAt: '2026-09-30T01:00:00+09:00', ops: 7, files: ['a', 'b'], tapeId: '20260930-0054-why-basic'}), 'row')

# --- commands and settings ---
t.Equal('x', ui.TapeId('x'), 'tape id')
t.Equal('20260930-0054-why-basic', ui.TapeId('.srwr/tapes/20260930-0054-why-basic.tape.jsonl'), 'tape path')
t.Equal('20260930-0054-why-basic', ui.TapeId('.srwr/tapes/20260930-0054-why-basic.tape.jsonl.gz'), 'a compressed tape path')
t.Equal('', ui.TapeId('  '), 'no tape')
t.Equal(t.Root() .. '/bin/srwr', config.Path(), 'path from g:srwr_path')
unlet g:srwr_path
t.Equal('srwr', config.Path(), 'default path')
t.Equal({diffFrames: true}, config.ServerOptions(), 'options')

t.Finish()
